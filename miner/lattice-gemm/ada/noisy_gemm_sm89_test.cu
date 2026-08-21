// End-to-end correctness test for the Ada (sm_89) NoisyGEMM.
//
// Mining is consensus-critical and fails silently: a kernel that is subtly
// wrong does not error, it produces proofs the verifier rejects, which presents
// as "mining does not work". So this checks the two things that have to be
// right, against references computed on the CPU rather than against the kernel
// itself:
//
//   The output tile. C = (A x B^T - EAL x EARxBpEB^T - AxEBL x EBR^T) scaled by
//   the per-row and per-column dequantisation factors, including the 2^-12
//   detour the fp16 denoise GEMM runs in. Checked with the tile exactly
//   covering the problem and again with M, N and K all indivisible by it, which
//   is what exercises the predication that replaced TMA's out-of-bounds
//   handling.
//
//   The transcript. Each thread folds its own accumulator every R/32 k-blocks
//   and races the result against the PoW target, so the transcript -- not just
//   the final C -- is what a nonce commits to. The CPU replays that fold for
//   all 256 threads, the repository's own BLAKE3 compresses the results, and
//   the target is set to the smallest hash so exactly one thread can win. If
//   the kernel's transcripts differ from the reference by even one bit, the
//   wrong thread wins, or none does.
//
// This one needs the GPU.
//
//   nvcc -std=c++20 -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
//     --expt-extended-lambda -DNDEBUG \
//     -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
//     -I csrc -I ada ada/noisy_gemm_sm89_test.cu -o /tmp/ada_test && /tmp/ada_test

#include <cmath>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <algorithm>
#include <vector>

#include <cuda_runtime.h>

#include "cute/tensor.hpp"

#include "blake3/blake3.cuh"
#include "lattice_gemm_sm89.h"

using namespace cute;

#define CHECK_CUDA(call)                                                    \
  do {                                                                      \
    cudaError_t err_ = (call);                                              \
    if (err_ != cudaSuccess) {                                              \
      printf("CUDA error %s at %s:%d\n", cudaGetErrorString(err_),          \
             __FILE__, __LINE__);                                           \
      return false;                                                         \
    }                                                                       \
  } while (0)

/// Compress transcripts with the repository's own BLAKE3, so the test checks
/// the Ada kernel rather than re-testing BLAKE3.
__global__ void hash_transcripts_kernel(uint32_t const* transcripts,
                                        uint32_t const* key, uint32_t* hashes,
                                        int count) {
  int const idx = blockIdx.x * blockDim.x + threadIdx.x;
  if (idx >= count) return;

  Tensor msg = make_tensor<uint32_t>(Int<blake3::MSG_BLOCK_SIZE_U32>{});
  for (int i = 0; i < blake3::MSG_BLOCK_SIZE_U32; ++i) {
    msg(i) = transcripts[idx * blake3::MSG_BLOCK_SIZE_U32 + i];
  }
  Tensor cv = make_tensor<uint32_t>(Int<blake3::CHAINING_VALUE_SIZE_U32>{});
  for (int i = 0; i < blake3::CHAINING_VALUE_SIZE_U32; ++i) {
    cv(i) = key[i];
  }
  blake3::compress_msg_block_u32(msg, cv,
                                 blake3::COMPRESS_PARAMS_SINGLE_BLOCK_KEYED);
  for (int i = 0; i < blake3::CHAINING_VALUE_SIZE_U32; ++i) {
    hashes[idx * blake3::CHAINING_VALUE_SIZE_U32 + i] = cv(i);
  }
}

namespace {

int failures = 0;

// ---------------------------------------------------------------------------
// Deterministic inputs
// ---------------------------------------------------------------------------

struct Rng {
  uint64_t state;
  explicit Rng(uint64_t seed) : state(seed) {}
  uint32_t next() {
    state += 0x9E3779B97F4A7C15ull;
    uint64_t z = state;
    z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9ull;
    z = (z ^ (z >> 27)) * 0x94D049BB133111EBull;
    return static_cast<uint32_t>(z ^ (z >> 31));
  }
  // MMAType::Int7xInt7ToInt32: operands are 7-bit signed.
  int8_t int7() { return static_cast<int8_t>(int(next() % 127) - 63); }
  float unit() { return float(next() % 2048) / 2048.0f - 0.5f; }
};

// ---------------------------------------------------------------------------
// References
// ---------------------------------------------------------------------------

/// C_acc = A x B^T, int32 with wraparound, exactly as the tensor core does it.
std::vector<int32_t> reference_accumulator(std::vector<int8_t> const& A,
                                           std::vector<int8_t> const& B, int M,
                                           int N, int K) {
  std::vector<int32_t> C(size_t(M) * N, 0);
  for (int m = 0; m < M; ++m) {
    for (int n = 0; n < N; ++n) {
      uint32_t acc = 0;
      for (int k = 0; k < K; ++k) {
        acc += uint32_t(int32_t(A[size_t(m) * K + k]) *
                        int32_t(B[size_t(n) * K + k]));
      }
      C[size_t(m) * N + n] = int32_t(acc);
    }
  }
  return C;
}

/// The epilogue's arithmetic, in the order the kernel performs it.
std::vector<float> reference_output(std::vector<int32_t> const& Cacc,
                                    std::vector<float> const& EAL,
                                    std::vector<float> const& EARxBpEB,
                                    std::vector<float> const& AxEBL,
                                    std::vector<float> const& EBR,
                                    std::vector<float> const& a_scales,
                                    std::vector<float> const& b_scales, int M,
                                    int N, int R, bool denoise) {
  std::vector<float> D(size_t(M) * N);
  float const s = float(lattice::kIntToFp16ScaleFactor);
  for (int m = 0; m < M; ++m) {
    for (int n = 0; n < N; ++n) {
      float d = float(Cacc[size_t(m) * N + n]);
      if (denoise) {
        d /= s;
        float y = 0.f, x = 0.f;
        for (int r = 0; r < R; ++r) {
          y += EAL[size_t(m) * R + r] * EARxBpEB[size_t(n) * R + r];
          x += AxEBL[size_t(m) * R + r] * EBR[size_t(n) * R + r];
        }
        d += y;
        d += x;
        d *= s;
      }
      D[size_t(m) * N + n] = d * a_scales[m] * b_scales[n];
    }
  }
  return D;
}

/// The tile-relative (row, column) pairs one thread accumulates.
///
/// Taken from the tiled MMA rather than written out, so the reference tracks
/// the kernel's actual partitioning. That the partitioning is the right one is
/// a separate question, settled by layout_equiv_sm89.cu.
template <class TiledMma>
std::vector<std::pair<int, int>> thread_coords(TiledMma mma, int thread_idx,
                                               int bM, int bN) {
  auto thr = mma.get_thread_slice(thread_idx);
  Tensor cD = make_identity_tensor(make_shape(bM, bN));
  Tensor tCcD = thr.partition_C(cD);
  std::vector<std::pair<int, int>> out;
  for (int i = 0; i < size(tCcD); ++i) {
    out.emplace_back(get<0>(tCcD(i)), get<1>(tCcD(i)));
  }
  return out;
}

/// Replay TileHashAccumulator for every thread of one tile.
///
/// The fold runs on the *running* accumulator, so this has to march through the
/// k-blocks in the kernel's order rather than working from the final C.
std::vector<std::array<uint32_t, 16>> reference_transcripts(
    std::vector<int8_t> const& A, std::vector<int8_t> const& B, int M, int N,
    int K, int bM, int bN, int bK, int R, int num_threads,
    std::vector<std::vector<std::pair<int, int>>> const& coords) {

  int const atom_k = lattice::ada::kMmaAtomK;
  int const k_blocks_per_tile = bK / atom_k;
  int const reduce_every_k = R / atom_k;
  int const accums_per_tile =
      std::max(1, k_blocks_per_tile / reduce_every_k);
  int const last_full_k_block = K / atom_k;
  int const k_tile_count = (K + bK - 1) / bK;

  std::vector<uint32_t> acc(size_t(bM) * bN, 0);  // the running accumulator
  std::vector<std::array<uint32_t, 16>> transcripts(
      num_threads, std::array<uint32_t, 16>{});

  int reduction_count = 0;
  int k_block_count = 0;

  for (int k_tile = 0; k_tile < k_tile_count; ++k_tile) {
    for (int kb = 0; kb < k_blocks_per_tile; ++kb) {
      int const k_begin = (k_tile * k_blocks_per_tile + kb) * atom_k;
      for (int m = 0; m < bM; ++m) {
        for (int n = 0; n < bN; ++n) {
          uint32_t partial = 0;
          for (int k = k_begin; k < k_begin + atom_k; ++k) {
            // Past the end of A, B or K the cp.async ZFILL wrote zeros, so
            // these terms contribute nothing -- the same as TMA's behaviour.
            if (m >= M || n >= N || k >= K) continue;
            partial += uint32_t(int32_t(A[size_t(m) * K + k]) *
                                int32_t(B[size_t(n) * K + k]));
          }
          acc[size_t(m) * bN + n] += partial;
        }
      }

      ++k_block_count;
      if ((k_block_count % reduce_every_k == 0) &&
          (k_block_count <= last_full_k_block)) {
        int const idx = kb / reduce_every_k;
        for (int t = 0; t < num_threads; ++t) {
          uint32_t h = 0;
          for (auto const& rc : coords[t]) {
            h ^= acc[size_t(rc.first) * bN + rc.second];
          }
          uint32_t& slot = transcripts[t][reduction_count + idx];
          slot = ((slot << lattice::HASH_ACCUMULATE_ROTATION) |
                  (slot >> (32 - lattice::HASH_ACCUMULATE_ROTATION))) ^ h;
        }
      }
    }
    if ((k_blocks_per_tile / reduce_every_k > 0) ||
        (k_block_count % reduce_every_k == 0)) {
      reduction_count = (reduction_count + accums_per_tile) % 16;
    }
  }
  return transcripts;
}

/// uint256 comparison, least significant word first.
bool hash_less(uint32_t const* a, uint32_t const* b) {
  for (int i = 7; i >= 0; --i) {
    if (a[i] != b[i]) return a[i] < b[i];
  }
  return false;
}

// ---------------------------------------------------------------------------
// Device buffers
// ---------------------------------------------------------------------------

template <typename T>
T* upload(std::vector<T> const& host) {
  T* ptr = nullptr;
  if (cudaMalloc(&ptr, host.size() * sizeof(T)) != cudaSuccess) return nullptr;
  if (cudaMemcpy(ptr, host.data(), host.size() * sizeof(T),
                 cudaMemcpyHostToDevice) != cudaSuccess) {
    return nullptr;
  }
  return ptr;
}

std::vector<cutlass::half_t> to_half(std::vector<float> const& v) {
  std::vector<cutlass::half_t> out(v.size());
  for (size_t i = 0; i < v.size(); ++i) out[i] = cutlass::half_t(v[i]);
  return out;
}

struct Problem {
  int M, N, K, R;
  std::vector<int8_t> A, B;
  std::vector<float> EAL, EARxBpEB, AxEBL, EBR, a_scales, b_scales;

  static Problem make(int M, int N, int K, int R, uint64_t seed) {
    Rng rng(seed);
    Problem p{M, N, K, R};
    p.A.resize(size_t(M) * K);
    p.B.resize(size_t(N) * K);
    for (auto& v : p.A) v = rng.int7();
    for (auto& v : p.B) v = rng.int7();
    // Kept small so the fp32 accumulation order inside the denoise GEMM does
    // not change the result beyond bfloat16's resolution.
    auto fill = [&](std::vector<float>& v, size_t n) {
      v.resize(n);
      for (auto& x : v) x = rng.unit() * 0.05f;
    };
    fill(p.EAL, size_t(M) * R);
    fill(p.EARxBpEB, size_t(N) * R);
    fill(p.AxEBL, size_t(M) * R);
    fill(p.EBR, size_t(N) * R);
    p.a_scales.resize(M);
    p.b_scales.resize(N);
    for (auto& x : p.a_scales) x = 0.5f + rng.unit();
    for (auto& x : p.b_scales) x = 0.5f + rng.unit();
    return p;
  }
};

}  // namespace

// ---------------------------------------------------------------------------
// Test 1: the output tile
// ---------------------------------------------------------------------------

template <bool Is_Even_M, bool Is_Even_N>
bool test_output(char const* name, int M, int N, int K, uint64_t seed) {
  using TileShape = Shape<_128, _256, _64, _128>;
  constexpr int R = 128;
  constexpr int kStages = 4;

  Problem p = Problem::make(M, N, K, R, seed);

  auto Cacc = reference_accumulator(p.A, p.B, M, N, K);
  // Round the denoise factors to fp16 before the reference uses them, so this
  // compares against what the kernel is actually given.
  auto round_half = [](std::vector<float> const& v) {
    std::vector<float> out(v.size());
    for (size_t i = 0; i < v.size(); ++i) {
      out[i] = float(cutlass::half_t(v[i]));
    }
    return out;
  };
  auto expected = reference_output(
      Cacc, round_half(p.EAL), round_half(p.EARxBpEB), round_half(p.AxEBL),
      round_half(p.EBR), p.a_scales, p.b_scales, M, N, R, /*denoise=*/true);

  auto hEAL = to_half(p.EAL), hEARxBpEB = to_half(p.EARxBpEB);
  auto hAxEBL = to_half(p.AxEBL), hEBR = to_half(p.EBR);

  int8_t* dA = upload(p.A);
  int8_t* dB = upload(p.B);
  auto* dEAL = upload(hEAL);
  auto* dEARxBpEB = upload(hEARxBpEB);
  auto* dAxEBL = upload(hAxEBL);
  auto* dEBR = upload(hEBR);
  float* dAscales = upload(p.a_scales);
  float* dBscales = upload(p.b_scales);

  std::vector<cutlass::bfloat16_t> hC(size_t(M) * N, cutlass::bfloat16_t(0.f));
  auto* dC = upload(hC);

  std::vector<uint32_t> target(8, 0u);  // unreachable: no thread should win
  std::vector<uint32_t> key(8, 0x12345678u);
  auto* dTarget = upload(target);
  auto* dKey = upload(key);

  void* dHeader = nullptr;
  void* dSync = nullptr;
  uint64_t* dCounter = nullptr;
  CHECK_CUDA(cudaMalloc(&dHeader, host_signal_header_size));
  CHECK_CUDA(cudaMalloc(&dSync, sizeof(HostSignalSync)));
  CHECK_CUDA(cudaMalloc(&dCounter, sizeof(uint64_t)));
  CHECK_CUDA(cudaMemset(dHeader, 0, host_signal_header_size));
  CHECK_CUDA(cudaMemset(dSync, 0, sizeof(HostSignalSync)));
  CHECK_CUDA(cudaMemset(dCounter, 0, sizeof(uint64_t)));

  lattice::ada::AdaGemmParams params{
      .ptr_ApEA = dA,
      .ptr_BpEB = dB,
      .ptr_C = dC,
      .ptr_A_scales = dAscales,
      .ptr_B_scales = dBscales,
      .ptr_EAL_mma = dEAL,
      .ptr_EARxBpEB_mma = dEARxBpEB,
      .ptr_AxEBL_mma = dAxEBL,
      .ptr_EBR_mma = dEBR,
      .host_signal_header_pinned = dHeader,
      .host_signal_sync = dSync,
      .inner_hash_counter = dCounter,
      .ptr_pow_target = dTarget,
      .ptr_pow_key = dKey,
      .m = M, .n = N, .k = K, .r = R,
      .swizzle = 1,
      .swizzle_n_maj = false};

  CHECK_CUDA((lattice::ada::run_lattice_gemm_sm89<
              cutlass::bfloat16_t, TileShape, kStages, Is_Even_M, Is_Even_N,
              /*SkipReduction=*/false, /*SkipDenoising=*/false>(params)));
  CHECK_CUDA(cudaDeviceSynchronize());
  CHECK_CUDA(cudaMemcpy(hC.data(), dC, hC.size() * sizeof(cutlass::bfloat16_t),
                        cudaMemcpyDeviceToHost));

  double max_rel = 0.0;
  int mismatched = 0;
  for (size_t i = 0; i < hC.size(); ++i) {
    double const got = double(float(hC[i]));
    double const want = double(expected[i]);
    double const denom = std::max(1.0, std::abs(want));
    double const rel = std::abs(got - want) / denom;
    max_rel = std::max(max_rel, rel);
    if (rel > 2e-2) ++mismatched;
  }

  printf("  %-28s %4d x %4d x %4d : %zu outputs, %d mismatched, max rel err %.3g\n",
         name, M, N, K, hC.size(), mismatched, max_rel);
  if (mismatched != 0) ++failures;

  for (void* ptr : {(void*)dA, (void*)dB, (void*)dC, (void*)dEAL,
                    (void*)dEARxBpEB, (void*)dAxEBL, (void*)dEBR,
                    (void*)dAscales, (void*)dBscales, (void*)dTarget,
                    (void*)dKey, dHeader, dSync, (void*)dCounter}) {
    cudaFree(ptr);
  }
  return mismatched == 0;
}

// ---------------------------------------------------------------------------
// Test 2: the transcript, through the PoW target
// ---------------------------------------------------------------------------

bool test_transcript(int K, uint64_t seed) {
  using TileShape = Shape<_128, _256, _64, _128>;
  constexpr int bM = 128, bN = 256, bK = 64, R = 128, kStages = 4;
  int const M = bM, N = bN;

  using Tiling = lattice::ada::AdaMmaTiling<bM, bN>;
  int const num_threads = Tiling::kNumMmaThreads;

  Problem p = Problem::make(M, N, K, R, seed);

  std::vector<std::vector<std::pair<int, int>>> coords(num_threads);
  for (int t = 0; t < num_threads; ++t) {
    coords[t] = thread_coords(typename Tiling::TiledMma{}, t, bM, bN);
  }

  auto transcripts = reference_transcripts(p.A, p.B, M, N, K, bM, bN, bK, R,
                                           num_threads, coords);

  std::vector<uint32_t> flat;
  flat.reserve(size_t(num_threads) * 16);
  for (auto const& t : transcripts) {
    flat.insert(flat.end(), t.begin(), t.end());
  }
  std::vector<uint32_t> key(8);
  {
    Rng rng(seed ^ 0xABCDEFull);
    for (auto& k : key) k = rng.next();
  }

  auto* dTranscripts = upload(flat);
  auto* dKey = upload(key);
  uint32_t* dHashes = nullptr;
  CHECK_CUDA(cudaMalloc(&dHashes, size_t(num_threads) * 8 * sizeof(uint32_t)));
  hash_transcripts_kernel<<<(num_threads + 127) / 128, 128>>>(
      dTranscripts, dKey, dHashes, num_threads);
  CHECK_CUDA(cudaDeviceSynchronize());

  std::vector<uint32_t> hashes(size_t(num_threads) * 8);
  CHECK_CUDA(cudaMemcpy(hashes.data(), dHashes, hashes.size() * sizeof(uint32_t),
                        cudaMemcpyDeviceToHost));

  int winner = 0;
  for (int t = 1; t < num_threads; ++t) {
    if (hash_less(&hashes[size_t(t) * 8], &hashes[size_t(winner) * 8])) {
      winner = t;
    }
  }
  int ties = 0;
  for (int t = 0; t < num_threads; ++t) {
    if (std::memcmp(&hashes[size_t(t) * 8], &hashes[size_t(winner) * 8],
                    8 * sizeof(uint32_t)) == 0) {
      ++ties;
    }
  }

  // Target = the smallest reference hash, so exactly one thread can win it.
  std::vector<uint32_t> target(hashes.begin() + size_t(winner) * 8,
                               hashes.begin() + size_t(winner) * 8 + 8);

  int8_t* dA = upload(p.A);
  int8_t* dB = upload(p.B);
  auto hEAL = to_half(p.EAL), hEARxBpEB = to_half(p.EARxBpEB);
  auto hAxEBL = to_half(p.AxEBL), hEBR = to_half(p.EBR);
  auto* dEAL = upload(hEAL);
  auto* dEARxBpEB = upload(hEARxBpEB);
  auto* dAxEBL = upload(hAxEBL);
  auto* dEBR = upload(hEBR);
  float* dAscales = upload(p.a_scales);
  float* dBscales = upload(p.b_scales);
  std::vector<cutlass::bfloat16_t> hC(size_t(M) * N, cutlass::bfloat16_t(0.f));
  auto* dC = upload(hC);
  auto* dTarget = upload(target);

  void* dHeader = nullptr;
  void* dSync = nullptr;
  uint64_t* dCounter = nullptr;
  CHECK_CUDA(cudaMallocHost(&dHeader, host_signal_header_size));
  CHECK_CUDA(cudaMalloc(&dSync, sizeof(HostSignalSync)));
  CHECK_CUDA(cudaMalloc(&dCounter, sizeof(uint64_t)));
  std::memset(dHeader, 0, host_signal_header_size);
  CHECK_CUDA(cudaMemset(dSync, 0, sizeof(HostSignalSync)));
  CHECK_CUDA(cudaMemset(dCounter, 0, sizeof(uint64_t)));

  lattice::ada::AdaGemmParams params{
      .ptr_ApEA = dA, .ptr_BpEB = dB, .ptr_C = dC,
      .ptr_A_scales = dAscales, .ptr_B_scales = dBscales,
      .ptr_EAL_mma = dEAL, .ptr_EARxBpEB_mma = dEARxBpEB,
      .ptr_AxEBL_mma = dAxEBL, .ptr_EBR_mma = dEBR,
      .host_signal_header_pinned = dHeader,
      .host_signal_sync = dSync,
      .inner_hash_counter = dCounter,
      .ptr_pow_target = dTarget, .ptr_pow_key = dKey,
      .m = M, .n = N, .k = K, .r = R,
      .swizzle = 1, .swizzle_n_maj = false};

  CHECK_CUDA((lattice::ada::run_lattice_gemm_sm89<
              cutlass::bfloat16_t, TileShape, kStages, true, true,
              /*SkipReduction=*/false, /*SkipDenoising=*/false>(params)));
  CHECK_CUDA(cudaDeviceSynchronize());

  auto const* header = reinterpret_cast<HostSignalHeader const*>(dHeader);
  bool ok = true;

  if (header->status != kSignalTriggered) {
    printf("  no thread reached the target -- the kernel's transcripts differ "
           "from the reference\n");
    ok = false;
  } else {
    std::vector<int> want_rows, want_cols;
    for (auto const& rc : coords[winner]) {
      want_rows.push_back(rc.first);
      want_cols.push_back(rc.second);
    }
    std::sort(want_rows.begin(), want_rows.end());
    want_rows.erase(std::unique(want_rows.begin(), want_rows.end()),
                    want_rows.end());
    std::sort(want_cols.begin(), want_cols.end());
    want_cols.erase(std::unique(want_cols.begin(), want_cols.end()),
                    want_cols.end());

    std::vector<int> got_rows, got_cols;
    for (int i = 0; i < header->num_registers_per_thread; ++i) {
      got_rows.push_back(header->thread_rows[i]);
      got_cols.push_back(header->thread_cols[i]);
    }
    std::sort(got_rows.begin(), got_rows.end());
    got_rows.erase(std::unique(got_rows.begin(), got_rows.end()),
                   got_rows.end());
    std::sort(got_cols.begin(), got_cols.end());
    got_cols.erase(std::unique(got_cols.begin(), got_cols.end()),
                   got_cols.end());

    printf("  winner: reference thread %d, kernel thread %u (%d tie%s)\n",
           winner, header->threadIdx[0], ties, ties == 1 ? "" : "s");
    printf("  registers reported: %u   rows: %zu   cols: %zu\n",
           header->num_registers_per_thread, got_rows.size(), got_cols.size());

    if (int(header->threadIdx[0]) != winner) {
      printf("  FAIL: a different thread reached the target\n");
      ok = false;
    }
    if (header->num_registers_per_thread != coords[winner].size()) {
      printf("  FAIL: expected %zu registers per thread\n",
             coords[winner].size());
      ok = false;
    }
    if (got_rows != want_rows || got_cols != want_cols) {
      printf("  FAIL: reported rows/columns do not match the winning thread\n");
      ok = false;
    }
    if (header->tileCoord[0] != 0 || header->tileCoord[1] != 0) {
      printf("  FAIL: unexpected tile coordinate\n");
      ok = false;
    }
  }

  if (!ok) ++failures;

  cudaFreeHost(dHeader);
  for (void* ptr : {(void*)dA, (void*)dB, (void*)dC, (void*)dEAL,
                    (void*)dEARxBpEB, (void*)dAxEBL, (void*)dEBR,
                    (void*)dAscales, (void*)dBscales, (void*)dTarget,
                    (void*)dKey, (void*)dTranscripts, (void*)dHashes, dSync,
                    (void*)dCounter}) {
    cudaFree(ptr);
  }
  return ok;
}

int main() {
  cudaDeviceProp props{};
  if (cudaGetDeviceProperties(&props, 0) != cudaSuccess) {
    printf("no CUDA device\n");
    return 1;
  }
  printf("Ada NoisyGEMM correctness on %s (sm_%d%d)\n", props.name,
         props.major, props.minor);
  printf("shared memory per block, opt-in: %zu B\n\n",
         props.sharedMemPerBlockOptin);

  printf("output tile:\n");
  test_output<true, true>("exact tile", 128, 256, 512, 1);
  test_output<true, true>("multiple tiles", 256, 512, 1024, 2);
  test_output<false, false>("M, N, K all remainders", 100, 200, 496, 3);

  printf("\ntranscript:\n");
  test_transcript(512, 11);

  printf("\n");
  if (failures == 0) {
    printf("PASS: the Ada kernel matches the reference\n");
    return 0;
  }
  printf("FAIL: %d check(s) failed\n", failures);
  return 1;
}

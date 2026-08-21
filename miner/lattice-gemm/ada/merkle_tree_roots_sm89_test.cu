// Correctness test for the Ada (sm_89) Merkle-roots kernel.
//
// Only the load path changed in that port: TMA staging became cp.async
// staging, and the producer warpgroup and the dual-pipeline mode went away with
// it. The hashing, the chunk flags and the Merkle reduction are the originals.
//
// So the reference here is a naive kernel with the same structure and no
// staging at all -- each thread reads its own chunk straight out of global
// memory, badly coalesced and slowly, and hashes it with the same blake3
// primitives and the same merkle_tree_utils reduction. Comparing the two
// isolates exactly what was ported: whether the cooperatively staged, swizzled
// shared-memory tile presents each thread with the bytes it would have read
// itself.
//
// Two shapes are checked: whole chunks filling every CTA, and a trailing
// partial chunk alone in the last CTA, which is what exercises the
// straight-from-global path for the final chunk.
//
// Needs the GPU.
//
//   nvcc -std=c++20 -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
//     --expt-extended-lambda -DNDEBUG \
//     -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
//     -I csrc -I ada ada/merkle_tree_roots_sm89_test.cu -o /tmp/mt_test && /tmp/mt_test

#include <cstdint>
#include <cstdio>
#include <cstring>
#include <vector>

#include <cuda_runtime.h>

#include "merkle_tree_roots_sm89.hpp"

using namespace cute;
namespace la = lattice::ada;

/// Reference: same structure, no staging. Every thread reads its own chunk
/// directly from global memory.
template <class Kernel>
__global__ void __launch_bounds__(Kernel::kNumThreads, 1)
    naive_merkle_tree_roots(typename Kernel::Params const params) {
  using u32 = uint32_t;
  constexpr int kNumThreads = Kernel::kNumThreads;

  __shared__ uint32_t leaves_storage[blake3::CHAINING_VALUE_SIZE_U32 *
                                     kNumThreads];
  Tensor sLeaves = make_tensor(
      make_smem_ptr(leaves_storage),
      make_layout(Shape<Int<blake3::CHAINING_VALUE_SIZE_U32>,
                        Int<kNumThreads>>{},
                  LayoutRight{}));

  int const tid = threadIdx.x;
  u32 const num_chunks = Kernel::compute_num_chunks(params.data_len);
  size_t const num_grid_blocks = (num_chunks + kNumThreads - 1) / kNumThreads;
  bool const is_single_chunk = (num_chunks == 1);
  u32 const global_chunk_idx = blockIdx.x * kNumThreads + tid;

  Tensor cv = make_tensor<uint32_t>(
      Layout<Shape<Int<blake3::CHAINING_VALUE_SIZE_U32>>>{});
  for (int i = 0; i < blake3::CHAINING_VALUE_SIZE_U32; ++i) cv(i) = c_key[i];

  Tensor rBlock =
      make_tensor<uint32_t>(Layout<Shape<Int<Kernel::kNumWordsPerBlock>>>{});

  for (int block_idx = 0; block_idx < Kernel::kNumBlocksPerChunk; ++block_idx) {
    for (int w = 0; w < Kernel::kNumWordsPerBlock; ++w) {
      size_t const byte =
          size_t(global_chunk_idx) * Kernel::kChunkSize +
          size_t(block_idx) * blake3::MSG_BLOCK_SIZE + size_t(w) * 4;
      uint32_t word = 0;
      // BLAKE3 zero-pads the trailing chunk.
      for (int b = 0; b < 4; ++b) {
        if (byte + b < params.data_len) {
          word |= uint32_t(params.ptr_data[byte + b]) << (8 * b);
        }
      }
      rBlock(w) = word;
    }

    blake3::CompressParams cp{.counter = global_chunk_idx,
                              .block_len = blake3::MSG_BLOCK_SIZE,
                              .flags = blake3::KEYED_HASH};
    if (block_idx == 0) cp.flags |= blake3::CHUNK_START;
    if (block_idx == Kernel::kNumBlocksPerChunk - 1) {
      cp.flags |= blake3::CHUNK_END;
      if (is_single_chunk && Kernel::kApplyRoot) cp.flags |= blake3::ROOT;
    }
    blake3::compress_msg_block_u32(rBlock, cv, cp);
  }

  for (int i = 0; i < blake3::CHAINING_VALUE_SIZE_U32; ++i) {
    sLeaves(i, tid) = cv(i);
  }
  __syncthreads();

  bool const is_last_block = (blockIdx.x == num_grid_blocks - 1);
  u32 const num_leaves = [&]() -> u32 {
    if (!is_last_block) return u32(kNumThreads);
    u32 const in_block = num_chunks % kNumThreads;
    return (in_block == 0) ? u32(kNumThreads) : in_block;
  }();

  if (!is_last_block) {
    lattice::merkle_tree_utils::compute_perfect_mt<false>(sLeaves, kNumThreads);
  } else if ((num_leaves & (num_leaves - 1)) == 0) {
    lattice::merkle_tree_utils::compute_perfect_mt<Kernel::kApplyRoot>(sLeaves,
                                                              num_leaves);
  } else {
    lattice::merkle_tree_utils::compute_blake_mt<Kernel::kApplyRoot>(sLeaves, num_leaves);
  }

  Tensor mRoots = make_tensor(
      reinterpret_cast<uint32_t*>(params.ptr_roots),
      make_layout(
          make_shape(Int<blake3::CHAINING_VALUE_SIZE_U32>{}, num_grid_blocks),
          make_stride(Int<1>{}, Int<blake3::CHAINING_VALUE_SIZE_U32>{})));
  if (tid < blake3::CHAINING_VALUE_SIZE_U32) {
    mRoots(tid, blockIdx.x) = sLeaves(tid, 0);
  }
}

namespace {

int failures = 0;

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
};

template <class Kernel>
bool run_case(char const* name, uint32_t data_len, uint64_t seed) {
  Rng rng(seed);
  std::vector<uint8_t> data(data_len);
  for (auto& b : data) b = uint8_t(rng.next());

  uint8_t* d_data = nullptr;
  if (cudaMalloc(&d_data, data.size()) != cudaSuccess) return false;
  cudaMemcpy(d_data, data.data(), data.size(), cudaMemcpyHostToDevice);

  uint32_t const num_chunks = Kernel::compute_num_chunks(data_len);
  uint32_t const num_grid_blocks =
      (num_chunks + Kernel::kNumThreads - 1) / Kernel::kNumThreads;
  size_t const roots_bytes = size_t(num_grid_blocks) * blake3::CHAINING_VALUE_SIZE;

  uint8_t* d_roots = nullptr;
  uint8_t* d_ref = nullptr;
  cudaMalloc(&d_roots, roots_bytes);
  cudaMalloc(&d_ref, roots_bytes);
  cudaMemset(d_roots, 0, roots_bytes);
  cudaMemset(d_ref, 0, roots_bytes);

  // A fixed key, uploaded the way tensor_hash_host.hpp does.
  uint32_t key[8];
  for (auto& k : key) k = rng.next();
  cudaMemcpyToSymbol(c_key, key, sizeof(key));

  typename Kernel::Arguments args{d_data, data_len, d_roots};
  cudaError_t err = la::run_merkle_tree_roots_sm89<Kernel>(args);
  if (err == cudaSuccess) err = cudaDeviceSynchronize();
  if (err != cudaSuccess) {
    printf("  %-38s CUDA error: %s\n", name, cudaGetErrorString(err));
    ++failures;
    return false;
  }

  typename Kernel::Params ref_params{d_data, data_len, d_ref};
  naive_merkle_tree_roots<Kernel>
      <<<num_grid_blocks, Kernel::kNumThreads>>>(ref_params);
  err = cudaDeviceSynchronize();
  if (err != cudaSuccess) {
    printf("  %-38s reference CUDA error: %s\n", name, cudaGetErrorString(err));
    ++failures;
    return false;
  }

  std::vector<uint8_t> got(roots_bytes), want(roots_bytes);
  cudaMemcpy(got.data(), d_roots, roots_bytes, cudaMemcpyDeviceToHost);
  cudaMemcpy(want.data(), d_ref, roots_bytes, cudaMemcpyDeviceToHost);

  int bad = 0;
  for (uint32_t b = 0; b < num_grid_blocks; ++b) {
    if (std::memcmp(&got[size_t(b) * blake3::CHAINING_VALUE_SIZE],
                    &want[size_t(b) * blake3::CHAINING_VALUE_SIZE],
                    blake3::CHAINING_VALUE_SIZE) != 0) {
      ++bad;
    }
  }

  printf("  %-38s %8u B, %u chunks, %u root%s : %d wrong\n", name, data_len,
         num_chunks, num_grid_blocks, num_grid_blocks == 1 ? "" : "s", bad);
  if (bad != 0) ++failures;

  cudaFree(d_data);
  cudaFree(d_roots);
  cudaFree(d_ref);
  return bad == 0;
}

}  // namespace

int main() {
  cudaDeviceProp props{};
  if (cudaGetDeviceProperties(&props, 0) != cudaSuccess) {
    printf("no CUDA device\n");
    return 1;
  }
  printf("Ada Merkle-roots correctness on %s (sm_%d%d)\n\n", props.name,
         props.major, props.minor);

  // Configurations that fit Ada's shared memory; see the static_assert in
  // merkle_tree_roots_sm89.hpp.
  using K256 = la::MerkleTreeRootsKernelSm89<256, 2, 128, /*kApplyRoot=*/false>;
  using K256s = la::MerkleTreeRootsKernelSm89<256, 3, 64, /*kApplyRoot=*/false>;
  using K512 = la::MerkleTreeRootsKernelSm89<512, 2, 64, /*kApplyRoot=*/false>;

  constexpr uint32_t kChunk = 1024;

  printf("256 threads, 128 B slices, 2 stages:\n");
  run_case<K256>("two full CTAs", 2 * 256 * kChunk, 1);
  run_case<K256>("full CTA plus one partial chunk", 256 * kChunk + 300, 2);
  run_case<K256>("one full CTA", 256 * kChunk, 3);

  printf("\n256 threads, 64 B slices, 3 stages:\n");
  run_case<K256s>("two full CTAs", 2 * 256 * kChunk, 4);
  run_case<K256s>("full CTA plus one partial chunk", 256 * kChunk + 17, 5);

  printf("\n512 threads, 64 B slices, 2 stages:\n");
  run_case<K512>("two full CTAs", 2 * 512 * kChunk, 6);
  run_case<K512>("full CTA plus one partial chunk", 512 * kChunk + 1000, 7);

  printf("\n");
  if (failures == 0) {
    printf("PASS: staged loads agree with direct global reads\n");
    return 0;
  }
  printf("FAIL: %d check(s) failed\n", failures);
  return 1;
}

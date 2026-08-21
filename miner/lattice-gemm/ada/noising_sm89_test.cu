// Correctness test for the Ada (sm_89) noising kernels.
//
// These have no consensus-visible thread layout the way the mainloop does --
// their outputs are dense matrices in global memory -- so a CPU reference
// settles them completely. What it has to get right is the two conversions,
// which are not the same:
//
//   int32 -> int8 on the noise term saturates (cvt.pack.sat.s8.s32), and
//   adding the input to it wraps, because that add happens on an int8 fragment.
//
// With 7-bit inputs and rank 128 the sum genuinely leaves int8's range, so a
// reference that saturated both steps, or wrapped both, would disagree.
//
// Both roles are checked -- the product on the raw input (noisingA) and on the
// freshly noised output (noisingB) -- along with the split-K path, whose
// atomicAdd replaced TMA's reduce-add, and with M and K left indivisible by the
// tile.
//
// Needs the GPU.
//
//   nvcc -std=c++20 -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
//     --expt-extended-lambda -DNDEBUG \
//     -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
//     -I csrc -I ada ada/noising_sm89_test.cu -o /tmp/noising_test && /tmp/noising_test

#include <cstdint>
#include <cstdio>
#include <algorithm>
#include <cmath>
#include <vector>

#include <cuda_runtime.h>

#include "noising_kernel_sm89.hpp"

using namespace cute;
namespace la = lattice::ada;

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
  int8_t int7() { return static_cast<int8_t>(int(next() % 127) - 63); }
};

std::vector<int8_t> random_int8(size_t n, uint64_t seed) {
  Rng rng(seed);
  std::vector<int8_t> v(n);
  for (auto& x : v) x = rng.int7();
  return v;
}

/// The noise term's int32 -> int8 conversion saturates.
int8_t saturate_int8(int32_t v) {
  return static_cast<int8_t>(std::clamp(v, -128, 127));
}

/// Adding the input happens on an int8 fragment, so it wraps.
int8_t wrap_int8(int a, int b) {
  return static_cast<int8_t>(static_cast<uint8_t>(a + b));
}

struct Reference {
  std::vector<int8_t> XpE;   // (M, K)
  std::vector<int32_t> XxP;  // (M, R), unscaled
};

Reference reference(std::vector<int8_t> const& X, std::vector<int8_t> const& NL,
                    std::vector<int8_t> const& NR, std::vector<int8_t> const& P,
                    int M, int K, int R, bool product_uses_noised) {
  Reference out;
  out.XpE.assign(size_t(M) * K, 0);
  out.XxP.assign(size_t(M) * R, 0);

  for (int m = 0; m < M; ++m) {
    for (int k = 0; k < K; ++k) {
      int32_t noise = 0;
      for (int r = 0; r < R; ++r) {
        noise += int32_t(NL[size_t(m) * R + r]) * int32_t(NR[size_t(k) * R + r]);
      }
      out.XpE[size_t(m) * K + k] =
          wrap_int8(int(X[size_t(m) * K + k]), int(saturate_int8(noise)));
    }
  }

  auto const& src = product_uses_noised ? out.XpE : X;
  for (int m = 0; m < M; ++m) {
    for (int r = 0; r < R; ++r) {
      uint32_t acc = 0;
      for (int k = 0; k < K; ++k) {
        acc += uint32_t(int32_t(src[size_t(m) * K + k]) *
                        int32_t(P[size_t(r) * K + k]));
      }
      out.XxP[size_t(m) * R + r] = int32_t(acc);
    }
  }
  return out;
}

template <typename T>
T* upload(std::vector<T> const& host) {
  T* ptr = nullptr;
  if (cudaMalloc(&ptr, host.size() * sizeof(T)) != cudaSuccess) return nullptr;
  cudaMemcpy(ptr, host.data(), host.size() * sizeof(T), cudaMemcpyHostToDevice);
  return ptr;
}

/// Run one configuration and compare both outputs against the reference.
template <class Kernel>
bool run_case(char const* name, int M, int K, int k_blocks_per_split,
              uint64_t seed) {
  constexpr int R = Kernel::R;
  constexpr int bK = Kernel::bK;
  using ElementDenoise = typename Kernel::ElementDenoise;
  constexpr bool product_uses_noised =
      Kernel::kProductOperand == la::ProductOperand::Noised;

  auto X = random_int8(size_t(M) * K, seed);
  auto NL = random_int8(size_t(M) * R, seed + 1);
  auto NR = random_int8(size_t(K) * R, seed + 2);
  auto P = random_int8(size_t(R) * K, seed + 3);

  Reference const ref = reference(X, NL, NR, P, M, K, R, product_uses_noised);

  int8_t* dX = upload(X);
  int8_t* dNL = upload(NL);
  int8_t* dNR = upload(NR);
  int8_t* dP = upload(P);

  std::vector<int8_t> hOut(size_t(M) * K, 0);
  int8_t* dOut = upload(hOut);
  std::vector<ElementDenoise> hXxP(size_t(M) * R, ElementDenoise(0));
  auto* dXxP = upload(hXxP);  // zeroed, as the reduction path requires

  int const total_k_blocks = (K + bK - 1) / bK;
  typename Kernel::Params params{
      .ptr_X = dX, .ptr_NL = dNL, .ptr_NR = dNR, .ptr_P = dP,
      .ptr_out = dOut, .ptr_XxP = dXxP,
      .m = M, .k = K,
      .num_k_blocks =
          Kernel::NoReduction ? total_k_blocks : k_blocks_per_split,
      .total_k_blocks = total_k_blocks};

  cudaError_t err = la::run_lattice_noising_sm89<Kernel>(params);
  if (err == cudaSuccess) err = cudaDeviceSynchronize();
  if (err != cudaSuccess) {
    printf("  %-34s CUDA error: %s\n", name, cudaGetErrorString(err));
    ++failures;
    return false;
  }

  cudaMemcpy(hOut.data(), dOut, hOut.size(), cudaMemcpyDeviceToHost);
  cudaMemcpy(hXxP.data(), dXxP, hXxP.size() * sizeof(ElementDenoise),
             cudaMemcpyDeviceToHost);

  int out_bad = 0;
  for (size_t i = 0; i < hOut.size(); ++i) {
    if (hOut[i] != ref.XpE[i]) ++out_bad;
  }

  int prod_bad = 0;
  double max_rel = 0.0;
  for (size_t i = 0; i < hXxP.size(); ++i) {
    if constexpr (cute::is_same_v<ElementDenoise, int32_t>) {
      if (int32_t(hXxP[i]) != ref.XxP[i]) ++prod_bad;
    } else {
      double const want = double(ref.XxP[i]) / double(Kernel::kOutputScaleFactor);
      double const got = double(float(hXxP[i]));
      double const rel = std::abs(got - want) / std::max(1.0, std::abs(want));
      max_rel = std::max(max_rel, rel);
      // fp16 keeps 11 bits of mantissa.
      if (rel > 1e-2) ++prod_bad;
    }
  }

  printf("  %-34s %4d x %4d : noised %d/%zu wrong, product %d/%zu wrong",
         name, M, K, out_bad, hOut.size(), prod_bad, hXxP.size());
  if (!cute::is_same_v<ElementDenoise, int32_t>) {
    printf(", max rel err %.3g", max_rel);
  }
  printf("\n");

  bool const ok = (out_bad == 0 && prod_bad == 0);
  if (!ok) ++failures;

  for (void* ptr : {(void*)dX, (void*)dNL, (void*)dNR, (void*)dP, (void*)dOut,
                    (void*)dXxP}) {
    cudaFree(ptr);
  }
  return ok;
}

}  // namespace

int main() {
  cudaDeviceProp props{};
  if (cudaGetDeviceProperties(&props, 0) != cudaSuccess) {
    printf("no CUDA device\n");
    return 1;
  }
  printf("Ada noising correctness on %s (sm_%d%d)\n\n", props.name, props.major,
         props.minor);

  using TileMRK = Shape<_64, _128, _64>;

  // noisingA: the rank-R product multiplies the raw input.
  using KA = la::NoisingKernelSm89<TileMRK, int8_t, cutlass::half_t, 3, true,
                                   true, true, la::ProductOperand::Raw,
                                   lattice::kAxEBLScaleFactor>;
  // noisingB: it multiplies the output the same kernel has just noised.
  using KB = la::NoisingKernelSm89<TileMRK, int8_t, cutlass::half_t, 3, true,
                                   true, true, la::ProductOperand::Noised,
                                   lattice::kEARxBpEBScaleFactor>;
  // Uneven M and K.
  using KAu = la::NoisingKernelSm89<TileMRK, int8_t, cutlass::half_t, 3, false,
                                    false, true, la::ProductOperand::Raw,
                                    lattice::kAxEBLScaleFactor>;
  using KBu = la::NoisingKernelSm89<TileMRK, int8_t, cutlass::half_t, 3, false,
                                    false, true, la::ProductOperand::Noised,
                                    lattice::kEARxBpEBScaleFactor>;
  // Split-K, reduced with atomicAdd into an int32 factor.
  using KAsplit = la::NoisingKernelSm89<TileMRK, int8_t, int32_t, 3, true, true,
                                        false, la::ProductOperand::Raw,
                                        lattice::kAxEBLScaleFactor>;
  using KBsplit = la::NoisingKernelSm89<TileMRK, int8_t, int32_t, 3, true, true,
                                        false, la::ProductOperand::Noised,
                                        lattice::kEARxBpEBScaleFactor>;

  printf("product on the raw input (noisingA):\n");
  run_case<KA>("exact tiles", 128, 512, 0, 21);
  run_case<KAu>("M and K remainders", 100, 496, 0, 22);
  run_case<KAsplit>("split-K, atomicAdd reduction", 128, 512, 2, 23);

  printf("\nproduct on the noised output (noisingB):\n");
  run_case<KB>("exact tiles", 128, 512, 0, 31);
  run_case<KBu>("M and K remainders", 100, 496, 0, 32);
  run_case<KBsplit>("split-K, atomicAdd reduction", 128, 512, 2, 33);

  printf("\n");
  if (failures == 0) {
    printf("PASS: the Ada noising kernels match the reference\n");
    return 0;
  }
  printf("FAIL: %d check(s) failed\n", failures);
  return 1;
}

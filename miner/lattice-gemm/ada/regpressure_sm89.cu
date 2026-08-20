// Register pressure of the Ada NoisyGEMM mainloop.
//
// The accumulator layout is fixed by consensus (see tiled_mma_sm89.hpp), which
// fixes the accumulator at 128 int32 registers per thread. Hopper can afford
// that because warpgroup MMA reads both operands straight out of shared memory
// and keeps no operand registers at all. Ada's SM80 atom cannot: A and B have
// to be staged in registers through ldmatrix, on top of those 128.
//
// If the total does not fit under 255 registers the mainloop spills to local
// memory, which on a bandwidth-bound int8 GEMM is not a slowdown to tune away
// later -- it is the difference between a working port and a pointless one. So
// this measures it before the mainloop is written.
//
// The kernel below is not the port. It is the mainloop's register working set:
// the full 128-register accumulator, ldmatrix staging of both operands for a
// whole k-block, 32 MMA instructions per k-block, and the transcript XOR fold
// from ../csrc/gemm/pow_utils.hpp. What it does not do is pipeline global loads,
// which costs a few more registers for cp.async addressing.
//
// ptxas reports register and shared-memory usage at compile time, so this needs
// no GPU -- read the -Xptxas -v output, the kernel is never launched. Built with
// the same flags setup.py passes:
//
//   nvcc -std=c++20 -x cu -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
//     --expt-extended-lambda --use_fast_math -DNDEBUG \
//     -DCUTLASS_DEBUG_TRACE_LEVEL=0 \
//     --ptxas-options=--verbose,--register-usage-level=10,--warn-on-local-memory-usage \
//     -I third_party/cutlass/include -I csrc \
//     -c ada/regpressure_sm89.cu -o /dev/null
//
//   Used 202 registers, 0 bytes spill stores, 0 bytes spill loads
//   64 bytes stack frame
//
// 202 of 255: it fits, and nothing spills. The 64-byte stack frame is the
// 16-word transcript, which is indexed by a running counter rather than a
// constant and so lives in local memory -- the same as on Hopper, and touched
// once per k-tile rather than in the inner loop.
//
// The consequence is occupancy: 202 registers x 256 threads = 51712 of the
// 65536 an Ada SM has, so one CTA per SM -- though the 98304 B of shared memory
// would have forced that anyway. That is the shape the production kernel
// already runs in: it declares __launch_bounds__(..., 1), and its 193.6 KB of
// shared memory leaves no room for a second CTA in Hopper's 228 KB either. So
// one CTA per SM is the expected operating point rather than a regression --
// but it does mean there is no register headroom to spend on unrolling the
// mainloop further.

#include "cute/tensor.hpp"

#include "gemm/pow_utils.hpp"
#include "tiled_mma_sm89.hpp"

using namespace cute;

namespace {

using Tiling = lattice::ada::AdaMmaTiling<128, 256>;

static constexpr int bM = Tiling::TileM;
static constexpr int bN = Tiling::TileN;
static constexpr int bK = 64;
static constexpr int kStages = 4;
static constexpr int kNoiseRank = 128;

// K-major, one stage after another. Unswizzled: the swizzle is a bank-conflict
// question, and it changes neither the register count nor the byte count.
using SmemLayoutA = decltype(tile_to_shape(
    Layout<Shape<_16, Int<bK>>, Stride<Int<bK>, _1>>{},
    Shape<Int<bM>, Int<bK>, Int<kStages>>{}));
using SmemLayoutB = decltype(tile_to_shape(
    Layout<Shape<_16, Int<bK>>, Stride<Int<bK>, _1>>{},
    Shape<Int<bN>, Int<bK>, Int<kStages>>{}));

struct SharedStorage {
  array_aligned<int8_t, cosize_v<SmemLayoutA>> smem_A;
  array_aligned<int8_t, cosize_v<SmemLayoutB>> smem_B;
};

}  // namespace

__global__ __launch_bounds__(Tiling::kNumMmaThreads, 1) void mainloop_working_set(
    int8_t const* __restrict__ ptr_A, int8_t const* __restrict__ ptr_B,
    int32_t* __restrict__ out, uint32_t* __restrict__ transcript_out,
    int k_tile_count) {

  extern __shared__ char smem[];
  auto& storage = *reinterpret_cast<SharedStorage*>(smem);

  Tensor sA = make_tensor(make_smem_ptr(storage.smem_A.data()), SmemLayoutA{});
  Tensor sB = make_tensor(make_smem_ptr(storage.smem_B.data()), SmemLayoutB{});

  // Fill shared memory once. The port pipelines this with cp.async; here it only
  // has to be something ptxas cannot fold away.
  for (int i = threadIdx.x; i < cosize_v<SmemLayoutA>; i += blockDim.x) {
    storage.smem_A[i] = ptr_A[i];
  }
  for (int i = threadIdx.x; i < cosize_v<SmemLayoutB>; i += blockDim.x) {
    storage.smem_B[i] = ptr_B[i];
  }
  __syncthreads();

  typename Tiling::TiledMma tiled_mma;
  auto thr_mma = tiled_mma.get_thread_slice(threadIdx.x);

  // The consensus-fixed accumulator: 2 rows x 64 columns per thread.
  Tensor tCrC = partition_fragment_C(tiled_mma, Shape<Int<bM>, Int<bN>>{});
  static_assert(size(tCrC) == Tiling::kAccumRegsPerThread);
  clear(tCrC);

  // ldmatrix staging of both operands.
  auto smem_tiled_copy_A =
      make_tiled_copy_A(Copy_Atom<SM75_U32x4_LDSM_N, int8_t>{}, tiled_mma);
  auto smem_thr_copy_A = smem_tiled_copy_A.get_thread_slice(threadIdx.x);
  auto smem_tiled_copy_B =
      make_tiled_copy_B(Copy_Atom<SM75_U32x2_LDSM_N, int8_t>{}, tiled_mma);
  auto smem_thr_copy_B = smem_tiled_copy_B.get_thread_slice(threadIdx.x);

  Tensor tCsA = smem_thr_copy_A.partition_S(sA);
  Tensor tCsB = smem_thr_copy_B.partition_S(sB);

  Tensor tCrA = thr_mma.partition_fragment_A(sA(_, _, _0{}));
  Tensor tCrB = thr_mma.partition_fragment_B(sB(_, _, _0{}));
  Tensor tCrA_view = smem_thr_copy_A.retile_D(tCrA);
  Tensor tCrB_view = smem_thr_copy_B.retile_D(tCrB);

  constexpr int k_blocks_per_tile = size<2>(tCrA);
  constexpr int reduce_every_k = kNoiseRank / lattice::ada::kMmaAtomK;

  Tensor transcript = make_tensor<uint32_t>(Int<16>{});
  clear(transcript);
  uint32_t k_block_count = 0;
  uint32_t reduction_count = 0;

  CUTLASS_PRAGMA_NO_UNROLL
  for (int k_tile = 0; k_tile < k_tile_count; ++k_tile) {
    int const stage = k_tile % kStages;

    CUTLASS_PRAGMA_UNROLL
    for (int k_block = 0; k_block < k_blocks_per_tile; ++k_block) {
      copy(smem_tiled_copy_A, tCsA(_, _, k_block, stage), tCrA_view(_, _, k_block));
      copy(smem_tiled_copy_B, tCsB(_, _, k_block, stage), tCrB_view(_, _, k_block));
      gemm(tiled_mma, tCrA(_, _, k_block), tCrB(_, _, k_block), tCrC);

      // The transcript fold, at the production cadence.
      ++k_block_count;
      if (k_block_count % reduce_every_k == 0) {
        uint32_t const hash = lattice::xor_reduction(tCrC);
        transcript(reduction_count) =
            lattice::rotl_xor<lattice::HASH_ACCUMULATE_ROTATION>(
                transcript(reduction_count), hash);
        reduction_count = (reduction_count + 1) % 16;
      }
    }
  }

  CUTLASS_PRAGMA_UNROLL
  for (int i = 0; i < size(tCrC); ++i) {
    out[threadIdx.x * size(tCrC) + i] = tCrC(i);
  }
  CUTLASS_PRAGMA_UNROLL
  for (int i = 0; i < 16; ++i) {
    transcript_out[threadIdx.x * 16 + i] = transcript(i);
  }
}

// Shared memory the launch would request, for comparison with the 101376 B
// budget worked out in smem_budget_sm89.cu.
static_assert(sizeof(SharedStorage) == kStages * (bM * bK + bN * bK));
static_assert(sizeof(SharedStorage) == 98304);

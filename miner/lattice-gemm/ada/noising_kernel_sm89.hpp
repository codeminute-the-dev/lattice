#pragma once

// Ada (sm_89) counterpart of ../csrc/gemm/lattice_noisingA_kernel.h and
// lattice_noisingB_kernel.h.
//
// Those two files are the same kernel written twice under different names. Each
// takes a matrix X, a pair of rank-R noise factors and a fourth matrix, and
// produces two outputs:
//
//     XpE = int8(X + saturate_int8(NL x NR^T))     the noised matrix
//     XxP = (X or XpE) x P^T                       a rank-R denoise factor
//
// with A -> B, EAL -> EBR, EAR -> EBL, EBL -> EAR, ApEA -> BpEB and
// AxEBL -> EARxBpEB. The one substantive difference is which matrix the second
// product multiplies: noisingA multiplies the raw A, noisingB the freshly
// noised BpEB. That is a template parameter here rather than a second file.
//
// The Hopper kernels are warp-specialised three ways -- one warpgroup issuing
// TMA, one computing the product, one computing the noised matrix -- with four
// pipelines between them, including one carrying BpEB from the warpgroup that
// produces it to the warpgroup that multiplies it. cp.async removes the reason
// for the producer warpgroup, and once the two consumers are serialised the
// dependency that pipeline existed to express becomes a barrier. So this is one
// pass over the k-blocks with every thread doing every part of it.
//
// Split-K reduction used TMA's SM90_TMA_REDUCE_ADD, which becomes atomicAdd.

#include <cutlass/cutlass.h>
#include <cutlass/fast_math.h>
#include <cutlass/numeric_types.h>

#include "cute/tensor.hpp"

#include "gemm/lattice_gemm_constants.hpp"
#include "gemm/utils.h"

#include "kernel_traits_sm89.hpp"
#include "tiled_mma_sm89.hpp"

namespace lattice::ada {

using namespace cute;

/// Which matrix the rank-R product multiplies: noisingA multiplies the raw
/// input, noisingB the noised output it has just computed.
enum class ProductOperand { Raw, Noised };

template <class TileShape_MRK_, class Element_, class ElementDenoise_,
          int kStages_, bool IsEvenM_, bool IsEvenK_, bool NoReduction_,
          ProductOperand kProductOperand_, int kOutputScaleFactor_>
struct NoisingKernelSm89 {

  using Element = Element_;
  using ElementDenoise = ElementDenoise_;
  using ElementAccum = int32_t;
  using ElementScale = float;
  static_assert(cute::is_same_v<Element, int8_t>);

  using TileShape_MRK = TileShape_MRK_;
  static constexpr int bM = get<0>(TileShape_MRK{});
  static constexpr int R = get<1>(TileShape_MRK{});
  static constexpr int bK = get<2>(TileShape_MRK{});

  static constexpr int kStages = kStages_;
  static constexpr bool IsEvenM = IsEvenM_;
  static constexpr bool IsEvenK = IsEvenK_;
  static constexpr bool NoReduction = NoReduction_;
  static constexpr ProductOperand kProductOperand = kProductOperand_;
  static constexpr int kOutputScaleFactor = kOutputScaleFactor_;

  static_assert(bM % 16 == 0, "bM must be a multiple of the atom's 16 rows");
  static_assert(bK % 32 == 0, "bK is an MMA K extent and a 16-byte row");
  static_assert(R % 32 == 0, "R is an MMA K extent and a 16-byte row");
  static_assert(kStages >= 2);

  // int32 output means split-K, which means the product is reduced with
  // atomicAdd and must not be scaled on the way out.
  static_assert(NoReduction || cute::is_same_v<ElementDenoise, int32_t>,
                "reduction requires an int32 accumulator in global memory");

  // ---------------------------------------------------------------------
  // MMA
  // ---------------------------------------------------------------------

  // Both products are int8 x int8 -> int32 with K-major operands, so one tiling
  // serves both: 4 warps down M to cover bM=64, 2 across N. cute tiles the rest
  // of the N extent -- bK for the noise product, R for the denoise factor.
  static constexpr int kWarpsM = bM / 16;
  static constexpr int kWarpsN = 2;
  static constexpr int kNumThreads = kWarpsM * kWarpsN * 32;
  using TiledMma = decltype(make_tiled_mma(
      MmaAtom{}, Layout<Shape<Int<kWarpsM>, Int<kWarpsN>, _1>>{}));

  // ---------------------------------------------------------------------
  // Shared memory
  // ---------------------------------------------------------------------

  using SmemLayoutX = decltype(tile_to_shape(
      SmemAtomK<Element, Int<bM>, Int<bK>>{},
      Shape<Int<bM>, Int<bK>, Int<kStages>>{}));
  using SmemLayoutP = decltype(tile_to_shape(
      SmemAtomK<Element, Int<R>, Int<bK>>{},
      Shape<Int<R>, Int<bK>, Int<kStages>>{}));
  using SmemLayoutNR = decltype(tile_to_shape(
      SmemAtomK<Element, Int<bK>, Int<R>>{},
      Shape<Int<bK>, Int<R>, Int<kStages>>{}));
  // Constant across k, so it is loaded once and not staged.
  using SmemLayoutNL = decltype(tile_to_shape(
      SmemAtomK<Element, Int<bM>, Int<R>>{}, Shape<Int<bM>, Int<R>>{}));
  // Single-buffered: written, consumed and stored within one k-block.
  using SmemLayoutOut = decltype(tile_to_shape(
      SmemAtomK<Element, Int<bM>, Int<bK>>{}, Shape<Int<bM>, Int<bK>>{}));
  using SmemLayoutXxP = decltype(tile_to_shape(
      SmemAtomK<ElementDenoise, Int<bM>, Int<R>>{}, Shape<Int<bM>, Int<R>>{}));

  struct SharedStorage : cute::aligned_struct<128> {
    union {
      struct {
        cute::array_aligned<Element, cute::cosize_v<SmemLayoutX>, 128> smem_X;
        cute::array_aligned<Element, cute::cosize_v<SmemLayoutP>, 128> smem_P;
        cute::array_aligned<Element, cute::cosize_v<SmemLayoutNR>, 128> smem_NR;
        cute::array_aligned<Element, cute::cosize_v<SmemLayoutNL>, 128> smem_NL;
        cute::array_aligned<Element, cute::cosize_v<SmemLayoutOut>, 128> smem_out;
      };
      // Only live after the k-loop is done with everything above.
      cute::array_aligned<ElementDenoise, cute::cosize_v<SmemLayoutXxP>, 128>
          smem_XxP;
    };
  };

  static constexpr int SharedStorageSize = sizeof(SharedStorage);

  // ---------------------------------------------------------------------
  // Copies
  // ---------------------------------------------------------------------

  static constexpr int kElemsPerLoad = 16;  // 16 B of int8
  template <int RowLen>
  using G2SCopy = decltype(make_tiled_copy(
      Copy_Atom<SM80_CP_ASYNC_CACHEGLOBAL_ZFILL<uint128_t>, Element>{},
      Layout<Shape<Int<kNumThreads / (RowLen / kElemsPerLoad)>,
                   Int<RowLen / kElemsPerLoad>>,
             Stride<Int<RowLen / kElemsPerLoad>, _1>>{},
      Layout<Shape<_1, Int<kElemsPerLoad>>>{}));

  using G2SCopyK = G2SCopy<bK>;  // rows of length bK: X and P
  using G2SCopyR = G2SCopy<R>;   // rows of length R:  NL and NR
  static_assert(kNumThreads % (bK / kElemsPerLoad) == 0);
  static_assert(kNumThreads % (R / kElemsPerLoad) == 0);

  using S2RCopyA = Copy_Atom<SM75_U32x4_LDSM_N, Element>;
  using S2RCopyB = Copy_Atom<SM75_U32x2_LDSM_N, Element>;

  static constexpr int kOutElemsPerStore = 16;
  using S2GCopyOut = decltype(make_tiled_copy(
      Copy_Atom<AutoVectorizingCopyWithAssumedAlignment<128>, Element>{},
      Layout<Shape<Int<kNumThreads / (bK / kOutElemsPerStore)>,
                   Int<bK / kOutElemsPerStore>>,
             Stride<Int<bK / kOutElemsPerStore>, _1>>{},
      Layout<Shape<_1, Int<kOutElemsPerStore>>>{}));

  static constexpr int kDenoiseElemsPerStore =
      128 / cutlass::sizeof_bits_v<ElementDenoise>;
  using S2GCopyXxP = decltype(make_tiled_copy(
      Copy_Atom<AutoVectorizingCopyWithAssumedAlignment<128>, ElementDenoise>{},
      Layout<Shape<Int<kNumThreads / (R / kDenoiseElemsPerStore)>,
                   Int<R / kDenoiseElemsPerStore>>,
             Stride<Int<R / kDenoiseElemsPerStore>, _1>>{},
      Layout<Shape<_1, Int<kDenoiseElemsPerStore>>>{}));

  // ---------------------------------------------------------------------
  // Arguments
  // ---------------------------------------------------------------------

  using ShapeT = cute::Shape<int32_t, int32_t>;
  using StrideT = cute::Stride<int32_t, _1>;
  using LayoutT = cute::Layout<ShapeT, StrideT>;

  struct Params {
    Element const* ptr_X;    // (M, K) K-major
    Element const* ptr_NL;   // (M, R) R-major
    Element const* ptr_NR;   // (K, R) R-major
    Element const* ptr_P;    // (R, K) K-major
    Element* ptr_out;        // (M, K) K-major
    ElementDenoise* ptr_XxP; // (M, R) R-major
    int m, k;
    int num_k_blocks;    // k-blocks per split
    int total_k_blocks;
  };

  static dim3 get_grid_shape(Params const& params) {
    int const m_blocks = cutlass::ceil_div(params.m, bM);
    if (NoReduction) {
      return dim3(m_blocks, 1, 1);
    }
    return dim3(m_blocks,
                cutlass::ceil_div(params.total_k_blocks, params.num_k_blocks),
                1);
  }

  static dim3 get_block_shape() { return dim3(kNumThreads, 1, 1); }
};

/// Fill a (CPY_M, CPY_K) predicate from a row bound and a column bound.
template <typename PredTensor, typename CoordTensor>
CUTLASS_DEVICE void fill_pred(PredTensor& pred, CoordTensor const& coord,
                              int row_bound, int col_offset, int col_bound) {
  CUTLASS_PRAGMA_UNROLL
  for (int k = 0; k < size<2>(coord); ++k) {
    CUTLASS_PRAGMA_UNROLL
    for (int m = 0; m < size<1>(coord); ++m) {
      pred(m, k) = (get<0>(coord(_0{}, m, k)) < row_bound) &&
                   (col_offset + get<1>(coord(_0{}, m, k)) < col_bound);
    }
  }
}

template <class Kernel>
__global__ void __launch_bounds__(Kernel::kNumThreads, 1)
    ada_noising(CUTE_GRID_CONSTANT typename Kernel::Params const params) {

  using Element = typename Kernel::Element;
  using ElementDenoise = typename Kernel::ElementDenoise;
  using LayoutT = typename Kernel::LayoutT;
  constexpr int bM = Kernel::bM;
  constexpr int bK = Kernel::bK;
  constexpr int R = Kernel::R;
  constexpr int kStages = Kernel::kStages;

  extern __shared__ char shared_memory[];
  auto& ss = *reinterpret_cast<typename Kernel::SharedStorage*>(shared_memory);

  int const tid = threadIdx.x;
  int const m_block = blockIdx.x;
  int const k_block_min = blockIdx.y * params.num_k_blocks;
  int const k_block_max =
      cute::min(k_block_min + params.num_k_blocks, params.total_k_blocks);
  int const residual_M = params.m - m_block * bM;

  LayoutT const layout_X =
      make_layout(make_shape(params.m, params.k), make_stride(params.k, _1{}));
  LayoutT const layout_NL =
      make_layout(make_shape(params.m, R), make_stride(R, _1{}));
  LayoutT const layout_NR =
      make_layout(make_shape(params.k, R), make_stride(R, _1{}));
  LayoutT const layout_P =
      make_layout(make_shape(R, params.k), make_stride(params.k, _1{}));
  LayoutT const layout_XxP =
      make_layout(make_shape(params.m, R), make_stride(R, _1{}));

  Tensor sX = make_tensor(make_smem_ptr(ss.smem_X.data()),
                          typename Kernel::SmemLayoutX{});
  Tensor sP = make_tensor(make_smem_ptr(ss.smem_P.data()),
                          typename Kernel::SmemLayoutP{});
  Tensor sNR = make_tensor(make_smem_ptr(ss.smem_NR.data()),
                           typename Kernel::SmemLayoutNR{});
  Tensor sNL = make_tensor(make_smem_ptr(ss.smem_NL.data()),
                           typename Kernel::SmemLayoutNL{});
  Tensor sOut = make_tensor(make_smem_ptr(ss.smem_out.data()),
                            typename Kernel::SmemLayoutOut{});

  Tensor mX = make_tensor(make_gmem_ptr(params.ptr_X), layout_X);
  Tensor mNL = make_tensor(make_gmem_ptr(params.ptr_NL), layout_NL);
  Tensor mNR = make_tensor(make_gmem_ptr(params.ptr_NR), layout_NR);
  Tensor mP = make_tensor(make_gmem_ptr(params.ptr_P), layout_P);

  Tensor gX = local_tile(mX, Shape<Int<bM>, Int<bK>>{}, make_coord(m_block, _));
  Tensor gNL =
      local_tile(mNL, Shape<Int<bM>, Int<R>>{}, make_coord(m_block, _0{}));
  Tensor gNR = local_tile(mNR, Shape<Int<bK>, Int<R>>{}, make_coord(_, _0{}));
  Tensor gP = local_tile(mP, Shape<Int<R>, Int<bK>>{}, make_coord(_0{}, _));

  // ---- staged loads --------------------------------------------------
  typename Kernel::G2SCopyK copy_k;
  typename Kernel::G2SCopyR copy_r;
  auto thr_copy_k = copy_k.get_thread_slice(tid);
  auto thr_copy_r = copy_r.get_thread_slice(tid);

  Tensor tXgX = thr_copy_k.partition_S(gX);
  Tensor tXsX = thr_copy_k.partition_D(sX);
  Tensor tPgP = thr_copy_k.partition_S(gP);
  Tensor tPsP = thr_copy_k.partition_D(sP);
  Tensor tNgN = thr_copy_r.partition_S(gNR);
  Tensor tNsN = thr_copy_r.partition_D(sNR);

  Tensor cX = make_identity_tensor(Shape<Int<bM>, Int<bK>>{});
  Tensor cP = make_identity_tensor(Shape<Int<R>, Int<bK>>{});
  Tensor cN = make_identity_tensor(Shape<Int<bK>, Int<R>>{});
  Tensor tXcX = thr_copy_k.partition_S(cX);
  Tensor tPcP = thr_copy_k.partition_S(cP);
  Tensor tNcN = thr_copy_r.partition_S(cN);

  Tensor pX = make_tensor<bool>(make_shape(size<1>(tXsX), size<2>(tXsX)));
  Tensor pP = make_tensor<bool>(make_shape(size<1>(tPsP), size<2>(tPsP)));
  Tensor pN = make_tensor<bool>(make_shape(size<1>(tNsN), size<2>(tNsN)));

  auto load_k_block = [&](int k_block, int stage) {
    if (k_block < k_block_max) {
      int const k_offset = k_block * bK;
      fill_pred(pX, tXcX, residual_M, k_offset, params.k);
      cute::copy_if(copy_k, pX, tXgX(_, _, _, k_block), tXsX(_, _, _, stage));
      // NR is (K, R): its rows are the k dimension, its columns are all in range.
      fill_pred(pN, tNcN, params.k - k_offset, 0, R);
      cute::copy_if(copy_r, pN, tNgN(_, _, _, k_block), tNsN(_, _, _, stage));
      // P is the product's second operand in both roles -- EBL for noisingA,
      // EAR for noisingB -- and is a different tensor from either noise factor,
      // so it is needed whichever matrix the product multiplies.
      fill_pred(pP, tPcP, R, k_offset, params.k);
      cute::copy_if(copy_k, pP, tPgP(_, _, _, k_block), tPsP(_, _, _, stage));
    }
    cute::cp_async_fence();
  };

  // NL does not change with k.
  {
    Tensor tLgL = thr_copy_r.partition_S(gNL);
    Tensor tLsL = thr_copy_r.partition_D(sNL);
    Tensor cL = make_identity_tensor(Shape<Int<bM>, Int<R>>{});
    Tensor tLcL = thr_copy_r.partition_S(cL);
    Tensor pL = make_tensor<bool>(make_shape(size<1>(tLsL), size<2>(tLsL)));
    fill_pred(pL, tLcL, residual_M, 0, R);
    cute::copy_if(copy_r, pL, tLgL, tLsL);
  }
  cute::cp_async_fence();

  CUTLASS_PRAGMA_UNROLL
  for (int s = 0; s < kStages - 1; ++s) {
    load_k_block(k_block_min + s, s);
  }

  // ---- MMA setup ------------------------------------------------------
  typename Kernel::TiledMma tiled_mma;
  auto thr_mma = tiled_mma.get_thread_slice(tid);

  auto s2r_A = make_tiled_copy_A(typename Kernel::S2RCopyA{}, tiled_mma);
  auto s2r_B = make_tiled_copy_B(typename Kernel::S2RCopyB{}, tiled_mma);
  auto s2r_thr_A = s2r_A.get_thread_slice(tid);
  auto s2r_thr_B = s2r_B.get_thread_slice(tid);

  // acc_XxP is carried across the whole k range; acc_E is per k-block.
  Tensor acc_XxP = partition_fragment_C(tiled_mma, Shape<Int<bM>, Int<R>>{});
  Tensor acc_E = partition_fragment_C(tiled_mma, Shape<Int<bM>, Int<bK>>{});
  Tensor acc_E_int8 = make_fragment_like<Element>(acc_E);
  clear(acc_XxP);

  // Where each of this thread's noise-accumulator registers lands in the tile,
  // so the input can be added to it and the result written back.
  Tensor cE = make_identity_tensor(Shape<Int<bM>, Int<bK>>{});
  Tensor tEcE = thr_mma.partition_C(cE);

  // ---- k-block loop ---------------------------------------------------
  CUTLASS_PRAGMA_NO_UNROLL
  for (int k_block = k_block_min; k_block < k_block_max; ++k_block) {
    cute::cp_async_wait<kStages - 2>();
    __syncthreads();

    int const stage = (k_block - k_block_min) % kStages;
    load_k_block(k_block + kStages - 1,
                 (k_block - k_block_min + kStages - 1) % kStages);

    // ---- noised matrix: XpE = int8(X + saturate_int8(NL x NR^T)) ----
    clear(acc_E);
    {
      Tensor sNR_stage = sNR(_, _, stage);
      Tensor tCsA = s2r_thr_A.partition_S(sNL);
      Tensor tCsB = s2r_thr_B.partition_S(sNR_stage);
      Tensor tCrA = thr_mma.partition_fragment_A(sNL);
      Tensor tCrB = thr_mma.partition_fragment_B(sNR_stage);
      Tensor tCrA_view = s2r_thr_A.retile_D(tCrA);
      Tensor tCrB_view = s2r_thr_B.retile_D(tCrB);
      CUTLASS_PRAGMA_NO_UNROLL
      for (int kb = 0; kb < size<2>(tCrA); ++kb) {
        cute::copy(s2r_A, tCsA(_, _, kb), tCrA_view(_, _, kb));
        cute::copy(s2r_B, tCsB(_, _, kb), tCrB_view(_, _, kb));
        cute::gemm(tiled_mma, tCrA(_, _, kb), tCrB(_, _, kb), acc_E);
      }
    }
    // Saturating int32 -> int8, matching the Hopper path exactly.
    lattice::convert_type_out(acc_E, acc_E_int8);

    // The input is added in int8, which wraps rather than saturating -- the
    // same as the Hopper kernel's `+=` on an int8 fragment. Note that the two
    // conversions differ: the noise term saturates on the way down from int32,
    // and only this add wraps.
    //
    // smem_out is single-buffered, and safe to overwrite here because the
    // barrier at the top of this iteration is after every read of it.
    CUTLASS_PRAGMA_UNROLL
    for (int i = 0; i < size(acc_E_int8); ++i) {
      int const m_idx = get<0>(tEcE(i));
      int const k_idx = get<1>(tEcE(i));
      int const sum = int(sX(m_idx, k_idx, stage)) + int(acc_E_int8(i));
      sOut(m_idx, k_idx) = static_cast<Element>(static_cast<uint8_t>(sum));
    }
    __syncthreads();

    // ---- store the noised matrix ------------------------------------
    {
      Tensor gOut = local_tile(
          make_tensor(make_gmem_ptr(params.ptr_out), layout_X),
          Shape<Int<bM>, Int<bK>>{}, make_coord(m_block, k_block));
      typename Kernel::S2GCopyOut s2g;
      auto thr_s2g = s2g.get_thread_slice(tid);
      Tensor tOsO = thr_s2g.partition_S(sOut);
      Tensor tOgO = thr_s2g.partition_D(gOut);
      Tensor cO = make_identity_tensor(Shape<Int<bM>, Int<bK>>{});
      Tensor tOcO = thr_s2g.partition_D(cO);
      int const k_offset = k_block * bK;
      CUTLASS_PRAGMA_UNROLL
      for (int n = 0; n < size<2>(tOgO); ++n) {
        CUTLASS_PRAGMA_UNROLL
        for (int m = 0; m < size<1>(tOgO); ++m) {
          bool const ok =
              (Kernel::IsEvenM || get<0>(tOcO(_0{}, m, n)) < residual_M) &&
              (Kernel::IsEvenK ||
               k_offset + get<1>(tOcO(_0{}, m, n)) < params.k);
          if (ok) cute::copy(s2g, tOsO(_, m, n), tOgO(_, m, n));
        }
      }
    }

    // ---- rank-R product ---------------------------------------------
    {
      Tensor sSrc = [&] {
        if constexpr (Kernel::kProductOperand == ProductOperand::Noised) {
          return sOut;
        } else {
          return sX(_, _, stage);
        }
      }();
      Tensor sP_stage = sP(_, _, stage);
      Tensor tCsA = s2r_thr_A.partition_S(sSrc);
      Tensor tCsB = s2r_thr_B.partition_S(sP_stage);
      Tensor tCrA = thr_mma.partition_fragment_A(sSrc);
      Tensor tCrB = thr_mma.partition_fragment_B(sP_stage);
      Tensor tCrA_view = s2r_thr_A.retile_D(tCrA);
      Tensor tCrB_view = s2r_thr_B.retile_D(tCrB);
      CUTLASS_PRAGMA_NO_UNROLL
      for (int kb = 0; kb < size<2>(tCrA); ++kb) {
        cute::copy(s2r_A, tCsA(_, _, kb), tCrA_view(_, _, kb));
        cute::copy(s2r_B, tCsB(_, _, kb), tCrB_view(_, _, kb));
        cute::gemm(tiled_mma, tCrA(_, _, kb), tCrB(_, _, kb), acc_XxP);
      }
    }
  }

  // ---- store the rank-R product --------------------------------------
  cute::cp_async_wait<0>();
  __syncthreads();  // smem_XxP overlaps everything the loop used

  Tensor sXxP = make_tensor(make_smem_ptr(ss.smem_XxP.data()),
                            typename Kernel::SmemLayoutXxP{});
  Tensor cXxP = make_identity_tensor(Shape<Int<bM>, Int<R>>{});
  Tensor tXcX2 = thr_mma.partition_C(cXxP);
  CUTLASS_PRAGMA_UNROLL
  for (int i = 0; i < size(acc_XxP); ++i) {
    int const m_idx = get<0>(tXcX2(i));
    int const r_idx = get<1>(tXcX2(i));
    if constexpr (cute::is_same_v<ElementDenoise, int32_t>) {
      sXxP(m_idx, r_idx) = acc_XxP(i);
    } else {
      sXxP(m_idx, r_idx) = static_cast<ElementDenoise>(
          static_cast<float>(acc_XxP(i)) /
          static_cast<float>(Kernel::kOutputScaleFactor));
    }
  }
  __syncthreads();

  {
    Tensor mXxP = make_tensor(make_gmem_ptr(params.ptr_XxP), layout_XxP);
    Tensor gXxP =
        local_tile(mXxP, Shape<Int<bM>, Int<R>>{}, make_coord(m_block, _0{}));
    typename Kernel::S2GCopyXxP s2g;
    auto thr_s2g = s2g.get_thread_slice(tid);
    Tensor tOsO = thr_s2g.partition_S(sXxP);
    Tensor tOgO = thr_s2g.partition_D(gXxP);
    Tensor cO = make_identity_tensor(Shape<Int<bM>, Int<R>>{});
    Tensor tOcO = thr_s2g.partition_D(cO);

    CUTLASS_PRAGMA_UNROLL
    for (int n = 0; n < size<2>(tOgO); ++n) {
      CUTLASS_PRAGMA_UNROLL
      for (int m = 0; m < size<1>(tOgO); ++m) {
        if (!Kernel::IsEvenM && get<0>(tOcO(_0{}, m, n)) >= residual_M) continue;
        if constexpr (Kernel::NoReduction) {
          cute::copy(s2g, tOsO(_, m, n), tOgO(_, m, n));
        } else {
          // TMA's reduce-add becomes an ordinary atomic: several k-splits
          // contribute to the same (M, R) tile.
          CUTLASS_PRAGMA_UNROLL
          for (int v = 0; v < size<0>(tOgO); ++v) {
            atomicAdd(&tOgO(v, m, n), tOsO(v, m, n));
          }
        }
      }
    }
  }
}

/// Launch one of the two noising kernels.
///
/// `ptr_XxP` must be zeroed beforehand when `NoReduction` is false: several
/// k-splits accumulate into the same (M, R) tile, exactly as they did through
/// TMA's reduce-add.
template <class Kernel>
cudaError_t run_lattice_noising_sm89(typename Kernel::Params const& params,
                                     cudaStream_t stream = 0) {
  // cp.async and the vector stores need 16-byte-aligned rows; see the note in
  // lattice_gemm_sm89.h.
  if (params.k % Kernel::kElemsPerLoad != 0 ||
      Kernel::R % Kernel::kElemsPerLoad != 0) {
    return cudaErrorInvalidValue;
  }
  if (params.num_k_blocks <= 0 || params.total_k_blocks <= 0) {
    return cudaErrorInvalidValue;
  }

  auto kernel = ada_noising<Kernel>;
  constexpr int smem_size = Kernel::SharedStorageSize;
  if constexpr (smem_size >= 48 * 1024) {
    cudaError_t const attr = cudaFuncSetAttribute(
        kernel, cudaFuncAttributeMaxDynamicSharedMemorySize, smem_size);
    if (attr != cudaSuccess) return attr;
  }

  kernel<<<Kernel::get_grid_shape(params), Kernel::get_block_shape(), smem_size,
           stream>>>(params);
  return cudaGetLastError();
}

}  // namespace lattice::ada

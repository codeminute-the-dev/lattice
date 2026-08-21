#pragma once

// Ada (sm_89) counterpart of ../csrc/gemm/kernel_traits.hpp.
//
// Two structural differences from the Hopper traits, both forced by the
// hardware rather than chosen:
//
//   No producer warpgroup. Hopper dedicates a warpgroup to issuing TMA and
//   keeps 2 more for MMA. cp.async has no such asymmetry -- every thread issues
//   its own loads -- so the CTA is exactly the MMA threads and the pipeline is
//   the ordinary SM80 multistage one: cp_async_fence / cp_async_wait<N> and a
//   __syncthreads, not an mbarrier pipeline.
//
//   Two-phase denoise storage. The Hopper SharedStorage holds all four denoise
//   factors at once, 192 KB at the production tile. collective_epilogue.hpp
//   consumes them as two strictly sequential GEMMs, so on Ada they share
//   storage instead, which is what brings the kernel inside 101376 B. See
//   smem_budget_sm89.cu.
//
// Everything the transcript depends on is unchanged: the MMA atom's K extent is
// still 32, and the accumulator's thread layout is the same map Hopper produces
// (layout_equiv_sm89.cu).

#include "cute/algorithm/copy.hpp"
#include "cute/atom/mma_atom.hpp"
#include "cute/tensor.hpp"

#include <cutlass/arch/arch.h>
#include "cutlass/cutlass.h"
#include "cutlass/gemm/collective/collective_builder.hpp"
#include "cutlass/layout/layout.h"
#include "cutlass/numeric_types.h"

#include "tiled_mma_sm89.hpp"

namespace lattice::ada {

using namespace cute;

/// Swizzled K-major shared-memory atom.
///
/// This is the same 32/64/128-byte swizzle family that ldmatrix wants, and
/// CUTLASS already has a selector that picks the right member for an element
/// type and K extent. It reaches it through a GMMA-named helper, but the result
/// is a plain cute layout -- no Hopper hardware is implied by using it.
template <class Element, class TileM, class TileK>
using SmemAtomK = decltype(cutlass::gemm::collective::detail::ss_smem_selector<
                           GMMA::Major::K, Element, TileM, TileK>());

template <typename ElementIn_, typename ElementOut_, typename ElementDenoise_,
          typename ElementScale_, typename TileShape_MNKR_, bool Is_Even_M_,
          bool Is_Even_N_, bool SkipReduction_, bool SkipDenoising_,
          int kStages_, bool EnableDebug_>
struct KernelTraitsSm89 {

  using ElementIn = ElementIn_;
  using ElementOut = ElementOut_;
  using ElementDenoise = ElementDenoise_;
  using ElementScale = ElementScale_;
  using ElementAccum = int32_t;
  using ElementDenoiseAccum = float;
  using index_t = int64_t;

  using TileShape_MNKR = TileShape_MNKR_;
  static constexpr bool Is_Even_M = Is_Even_M_;
  static constexpr bool Is_Even_N = Is_Even_N_;
  static constexpr bool SkipReduction = SkipReduction_;
  static constexpr bool SkipDenoising = SkipDenoising_;
  static constexpr int kStages = kStages_;
  static constexpr bool EnableDebug = EnableDebug_;

  using ProblemShape = Shape<int, int, int, int>;
  static_assert(is_same_v<ElementDenoise, half_t>,
                "the Ada denoise GEMM is fp16; int32 factors go through the "
                "denoise conversion kernel first");

  static constexpr int bM = get<0>(TileShape_MNKR{});
  static constexpr int bN = get<1>(TileShape_MNKR{});
  static constexpr int bK = get<2>(TileShape_MNKR{});
  static constexpr int R = get<3>(TileShape_MNKR{});

  using TileShape_MNK = Shape<Int<bM>, Int<bN>, Int<bK>>;
  using TileShape_MNR = Shape<Int<bM>, Int<bN>, Int<R>>;

  // Ada has no clusters. Kept as a type so the shared pieces of the host code
  // and the tile scheduler read the same as the Hopper ones.
  using ClusterShape_MNK = Shape<_1, _1, _1>;

  // ---------------------------------------------------------------------
  // MMA
  // ---------------------------------------------------------------------

  using MmaTiling = AdaMmaTiling<bM, bN>;
  using TiledMma = typename MmaTiling::TiledMma;
  using MMAAtom_K = Int<kMmaAtomK>;

  static constexpr int kNumMmaThreads = MmaTiling::kNumMmaThreads;
  static constexpr int kNumThreads = kNumMmaThreads;
  static constexpr int kNumWarps = kNumThreads / cutlass::NumThreadsPerWarp;
  static constexpr int kNumMmaWarps = MmaTiling::kNumWarps;

  // fp16 denoise GEMM, tiled the same way. Its accumulator lands in the same
  // registers as the int8 mainloop's, which is what lets the denoise GEMMs
  // accumulate straight onto the converted mainloop result.
  using TiledMmaDenoise = decltype(make_tiled_mma(
      SM80_16x8x16_F32F16F16F32_TN{},
      Layout<Shape<Int<kNumMmaWarps>, _1, _1>>{}));
  static constexpr int kDenoiseAtomK = 16;
  static_assert(R % kDenoiseAtomK == 0);
  static_assert(bK % kMmaAtomK == 0);

  // ---------------------------------------------------------------------
  // Shared memory layouts
  // ---------------------------------------------------------------------

  using SmemLayoutAtomA = SmemAtomK<ElementIn, Int<bM>, Int<bK>>;
  using SmemLayoutA = decltype(tile_to_shape(
      SmemLayoutAtomA{}, Shape<Int<bM>, Int<bK>, Int<kStages>>{}));

  using SmemLayoutAtomB = SmemAtomK<ElementIn, Int<bN>, Int<bK>>;
  using SmemLayoutB = decltype(tile_to_shape(
      SmemLayoutAtomB{}, Shape<Int<bN>, Int<bK>, Int<kStages>>{}));

  using SmemLayoutAtomC = SmemAtomK<ElementOut, Int<bM>, Int<bN>>;
  using SmemLayoutC =
      decltype(tile_to_shape(SmemLayoutAtomC{}, select<0, 1>(TileShape_MNK{})));

  // Denoise factors, one stage each: they are loaded once per output tile.
  using SmemLayoutAtomDenoiseM = SmemAtomK<ElementDenoise, Int<bM>, Int<R>>;
  using SmemLayoutAtomDenoiseN = SmemAtomK<ElementDenoise, Int<bN>, Int<R>>;
  using SmemLayoutEAL = decltype(tile_to_shape(SmemLayoutAtomDenoiseM{},
                                               Shape<Int<bM>, Int<R>>{}));
  using SmemLayoutAxEBL = decltype(tile_to_shape(SmemLayoutAtomDenoiseM{},
                                                 Shape<Int<bM>, Int<R>>{}));
  using SmemLayoutEARxBpEB = decltype(tile_to_shape(SmemLayoutAtomDenoiseN{},
                                                    Shape<Int<bN>, Int<R>>{}));
  using SmemLayoutEBR = decltype(tile_to_shape(SmemLayoutAtomDenoiseN{},
                                               Shape<Int<bN>, Int<R>>{}));

  using SmemLayoutScaleA = Layout<Shape<Int<bM>>, Stride<_1>>;
  using SmemLayoutScaleB = Layout<Shape<Int<bN>>, Stride<_1>>;

  // ---------------------------------------------------------------------
  // Copy atoms
  // ---------------------------------------------------------------------

  // GMEM -> SMEM. The ZFILL variant writes zeros when the predicate is false,
  // which is how TMA handles an out-of-bounds tile on Hopper; the mainloop
  // relies on that for the M, N and K remainders.
  static constexpr int kGmemCopyBits = 128;
  static constexpr int kGmemElemsPerLoadIn =
      kGmemCopyBits / cutlass::sizeof_bits_v<ElementIn>;
  static constexpr int kGmemElemsPerLoadDenoise =
      kGmemCopyBits / cutlass::sizeof_bits_v<ElementDenoise>;

  using G2SCopyAtomIn =
      Copy_Atom<SM80_CP_ASYNC_CACHEGLOBAL_ZFILL<uint128_t>, ElementIn>;
  using G2SCopyAtomDenoise =
      Copy_Atom<SM80_CP_ASYNC_CACHEGLOBAL_ZFILL<uint128_t>, ElementDenoise>;

  // Threads are laid out so that each one loads a contiguous 16B along K.
  static constexpr int kGmemThreadsPerRowIn = bK / kGmemElemsPerLoadIn;
  static constexpr int kGmemThreadsPerRowDenoise =
      R / kGmemElemsPerLoadDenoise;
  static_assert(kNumThreads % kGmemThreadsPerRowIn == 0);
  static_assert(kNumThreads % kGmemThreadsPerRowDenoise == 0);

  using G2STiledCopyIn = decltype(make_tiled_copy(
      G2SCopyAtomIn{},
      Layout<Shape<Int<kNumThreads / kGmemThreadsPerRowIn>,
                   Int<kGmemThreadsPerRowIn>>,
             Stride<Int<kGmemThreadsPerRowIn>, _1>>{},
      Layout<Shape<_1, Int<kGmemElemsPerLoadIn>>>{}));

  using G2STiledCopyDenoise = decltype(make_tiled_copy(
      G2SCopyAtomDenoise{},
      Layout<Shape<Int<kNumThreads / kGmemThreadsPerRowDenoise>,
                   Int<kGmemThreadsPerRowDenoise>>,
             Stride<Int<kGmemThreadsPerRowDenoise>, _1>>{},
      Layout<Shape<_1, Int<kGmemElemsPerLoadDenoise>>>{}));

  // SMEM -> RMEM. Both atoms put half as many values in a B fragment as in an
  // A fragment -- 16 bytes against 32 for int8, 8 against 16 for fp16 -- so the
  // B side takes the x2 variant of ldmatrix and the A side the x4.
  using S2RCopyAtomA = Copy_Atom<SM75_U32x4_LDSM_N, ElementIn>;
  using S2RCopyAtomB = Copy_Atom<SM75_U32x2_LDSM_N, ElementIn>;
  using S2RCopyAtomDenoiseA = Copy_Atom<SM75_U32x4_LDSM_N, ElementDenoise>;
  using S2RCopyAtomDenoiseB = Copy_Atom<SM75_U32x2_LDSM_N, ElementDenoise>;

  // Hopper stages the output tile with stmatrix, which arrived with sm_90. The
  // epilogue here writes the tile straight into shared memory as it applies the
  // scales, so there is no RMEM -> SMEM copy atom to replace it with.
  //
  // SMEM -> GMEM for the output tile, replacing the TMA store.
  static constexpr int kGmemElemsPerStoreOut =
      kGmemCopyBits / cutlass::sizeof_bits_v<ElementOut>;
  static constexpr int kGmemThreadsPerRowOut = bN / kGmemElemsPerStoreOut;
  static_assert(kNumThreads % kGmemThreadsPerRowOut == 0);
  using S2GTiledCopyC = decltype(make_tiled_copy(
      Copy_Atom<AutoVectorizingCopyWithAssumedAlignment<128>, ElementOut>{},
      Layout<Shape<Int<kNumThreads / kGmemThreadsPerRowOut>,
                   Int<kGmemThreadsPerRowOut>>,
             Stride<Int<kGmemThreadsPerRowOut>, _1>>{},
      Layout<Shape<_1, Int<kGmemElemsPerStoreOut>>>{}));

  // Scales, one element per row or column.
  using G2SScales_copy_atom =
      Copy_Atom<Copy_Traits<SM80_CP_ASYNC_CACHEALWAYS<ElementScale>>,
                ElementScale>;
  using G2SScalesCopyA = decltype(make_tiled_copy(
      G2SScales_copy_atom{}, Layout<Shape<Int<bM>>, Stride<_1>>{},
      Layout<Shape<_1>, Stride<_1>>{}));
  using G2SScalesCopyB = decltype(make_tiled_copy(
      G2SScales_copy_atom{}, Layout<Shape<Int<bN>>, Stride<_1>>{},
      Layout<Shape<_1>, Stride<_1>>{}));

  // ---------------------------------------------------------------------
  // Shared storage
  // ---------------------------------------------------------------------

  struct SharedStorage : cute::aligned_struct<128> {
    union {
      // The mainloop's staged A and B.
      struct {
        cute::array_aligned<ElementIn, cute::cosize_v<SmemLayoutA>,
                            cutlass::detail::alignment_for_swizzle(
                                SmemLayoutA{})>
            smem_A;
        cute::array_aligned<ElementIn, cute::cosize_v<SmemLayoutB>,
                            cutlass::detail::alignment_for_swizzle(
                                SmemLayoutB{})>
            smem_B;
      };

      // Denoise phase 1: Y = -EAL x EARxBpEB.
      struct {
        cute::array_aligned<ElementDenoise, cute::cosize_v<SmemLayoutEAL>,
                            cutlass::detail::alignment_for_swizzle(
                                SmemLayoutEAL{})>
            smem_EAL;
        cute::array_aligned<ElementDenoise, cute::cosize_v<SmemLayoutEARxBpEB>,
                            cutlass::detail::alignment_for_swizzle(
                                SmemLayoutEARxBpEB{})>
            smem_EARxBpEB;
      };

      // Denoise phase 2: X = -AxEBL x EBR. Overlaps phase 1, which has been
      // consumed by the time these are loaded.
      struct {
        cute::array_aligned<ElementDenoise, cute::cosize_v<SmemLayoutAxEBL>,
                            cutlass::detail::alignment_for_swizzle(
                                SmemLayoutAxEBL{})>
            smem_AxEBL;
        cute::array_aligned<ElementDenoise, cute::cosize_v<SmemLayoutEBR>,
                            cutlass::detail::alignment_for_swizzle(
                                SmemLayoutEBR{})>
            smem_EBR;
      };

      // Staged output tile. Overlaps everything above: by the time C is
      // written the tile's A, B and denoise factors are all finished with.
      cute::array_aligned<ElementOut, cute::cosize_v<SmemLayoutC>,
                          cutlass::detail::alignment_for_swizzle(SmemLayoutC{})>
          smem_C;
    };

    cute::array_aligned<ElementScale, cute::cosize_v<SmemLayoutScaleA>> smem_scale_a;
    cute::array_aligned<ElementScale, cute::cosize_v<SmemLayoutScaleB>> smem_scale_b;
  };

  static constexpr int kSmemSize = sizeof(SharedStorage);
};

}  // namespace lattice::ada

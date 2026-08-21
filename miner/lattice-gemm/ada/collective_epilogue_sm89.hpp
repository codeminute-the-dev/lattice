#pragma once

// Ada (sm_89) counterpart of ../csrc/gemm/collective_epilogue.hpp.
//
// Three changes, all of them consequences of the missing hardware:
//
//   The denoise factors are loaded in two phases rather than all at once. They
//   were never used at once -- the Hopper epilogue runs Y = -EAL x EARxBpEB to
//   completion and releases its pipeline before it so much as waits on
//   AxEBL x EBR -- but they were all resident, and 192 KB does not fit in Ada's
//   101376 B. Sharing the storage costs the overlap between the two GEMMs and
//   is what makes the kernel fit at all (smem_budget_sm89.cu).
//
//   No load pipeline. With the phases serialised there is nothing left to
//   overlap, so the mbarrier pipelines collapse into cp.async plus a barrier.
//
//   No TMA store. The output tile is staged through shared memory as before,
//   then written with predicated vector stores.
//
// The arithmetic is unchanged, including the 2^-12 scaling the fp16 denoise
// GEMM runs in and the order of the two subtractions.

#include <cutlass/cutlass.h>
#include <cutlass/numeric_types.h>

#include "cute/tensor.hpp"

#include "gemm/lattice_gemm_constants.hpp"

#include "kernel_traits_sm89.hpp"

namespace lattice::ada {

using namespace cute;

template <typename KTraits>
struct CollectiveEpilogueSm89 {

  using ElementOutput = typename KTraits::ElementOut;
  using ElementDenoise = typename KTraits::ElementDenoise;
  using ElementScale = typename KTraits::ElementScale;

  using ProblemShape = typename KTraits::ProblemShape;
  using TileShape_MNK = typename KTraits::TileShape_MNK;
  using TileShape_MNR = typename KTraits::TileShape_MNR;
  static constexpr int bM = KTraits::bM;
  static constexpr int bN = KTraits::bN;
  static constexpr int R = KTraits::R;

  using SharedStorage = typename KTraits::SharedStorage;
  using SmemLayoutC = typename KTraits::SmemLayoutC;
  using SmemLayoutScaleA = typename KTraits::SmemLayoutScaleA;
  using SmemLayoutScaleB = typename KTraits::SmemLayoutScaleB;
  using SmemLayoutEAL = typename KTraits::SmemLayoutEAL;
  using SmemLayoutEARxBpEB = typename KTraits::SmemLayoutEARxBpEB;
  using SmemLayoutAxEBL = typename KTraits::SmemLayoutAxEBL;
  using SmemLayoutEBR = typename KTraits::SmemLayoutEBR;

  using Layout1DT = Layout<Shape<int32_t>, Stride<_1>>;
  using Shape2DT = Shape<int32_t, int32_t>;
  using Stride2DT = Stride<int32_t, _1>;
  using Layout2DT = Layout<Shape2DT, Stride2DT>;
  using ShapeDenoiseT = Shape<int, Int<R>>;
  using StrideDenoiseT = Stride<Int<R>, _1>;
  using LayoutDenoiseT = Layout<ShapeDenoiseT, StrideDenoiseT>;

  struct Arguments {
    ElementOutput* ptr_C;
    ElementScale const* ptr_A_scales;
    ElementScale const* ptr_B_scales;
    ElementDenoise const* ptr_EAL;
    ElementDenoise const* ptr_EARxBpEB;
    ElementDenoise const* ptr_AxEBL;
    ElementDenoise const* ptr_EBR;
    ProblemShape problem_shape;
  };

  struct Params {
    ElementOutput* ptr_C;
    ElementDenoise const* EAL;
    ElementDenoise const* EARxBpEB;
    ElementDenoise const* AxEBL;
    ElementDenoise const* EBR;
    ElementScale const* ptr_A_scales;
    ElementScale const* ptr_B_scales;
    Layout2DT layout_C;
    Layout1DT layout_A_scales;
    Layout1DT layout_B_scales;
    LayoutDenoiseT layout_M_denoise;
    LayoutDenoiseT layout_N_denoise;
    ProblemShape problem_shape;
  };

  static Params to_underlying_arguments(Arguments const& args) {
    auto [M, N, K, R_] = args.problem_shape;
    return {.ptr_C = args.ptr_C,
            .EAL = args.ptr_EAL,
            .EARxBpEB = args.ptr_EARxBpEB,
            .AxEBL = args.ptr_AxEBL,
            .EBR = args.ptr_EBR,
            .ptr_A_scales = args.ptr_A_scales,
            .ptr_B_scales = args.ptr_B_scales,
            .layout_C = make_layout(make_shape(M, N), make_stride(N, _1{})),
            .layout_A_scales = make_layout(make_shape(M)),
            .layout_B_scales = make_layout(make_shape(N)),
            .layout_M_denoise =
                make_layout(make_shape(M, Int<R>{}), Stride<Int<R>, _1>{}),
            .layout_N_denoise =
                make_layout(make_shape(N, Int<R>{}), Stride<Int<R>, _1>{}),
            .problem_shape = args.problem_shape};
  }

 private:
  /// cp.async one (bMN x R) denoise factor into shared memory, zero-filling the
  /// rows past the end of the problem the way TMA would have.
  template <typename SmemTensor>
  CUTLASS_DEVICE static void load_denoise_factor(
      ElementDenoise const* ptr, LayoutDenoiseT const& layout,
      SmemTensor& sX, int mn_block, int residual_mn, int thread_idx) {

    Tensor mX = make_tensor(make_gmem_ptr(ptr), layout);
    Tensor gX = local_tile(mX, Shape<Int<size<0>(SmemTensor{})>, Int<R>>{},
                           make_coord(mn_block, _0{}));

    typename KTraits::G2STiledCopyDenoise g2s_copy;
    auto thr_copy = g2s_copy.get_thread_slice(thread_idx);
    Tensor tXgX = thr_copy.partition_S(gX);
    Tensor tXsX = thr_copy.partition_D(sX);

    Tensor cX = make_identity_tensor(
        Shape<Int<size<0>(SmemTensor{})>, Int<R>>{});
    Tensor tXcX = thr_copy.partition_S(cX);
    Tensor tXpX = make_tensor<bool>(make_shape(size<1>(tXsX), size<2>(tXsX)));
    CUTLASS_PRAGMA_UNROLL
    for (int k = 0; k < size<2>(tXcX); ++k) {
      CUTLASS_PRAGMA_UNROLL
      for (int m = 0; m < size<1>(tXcX); ++m) {
        tXpX(m, k) = get<0>(tXcX(_0{}, m, k)) < residual_mn;
      }
    }
    cute::copy_if(g2s_copy, tXpX, tXgX, tXsX);
  }

  /// One rank-R correction: tCrD += -(A_factor x B_factor^T), fp16 in, fp32 out.
  ///
  /// The k-block is sliced out of shared memory before the operand fragments
  /// are built, rather than after. Building them over the whole rank first and
  /// indexing a k-block out of them costs 512 registers on the B side alone --
  /// measured, not guessed: it spills 1128 bytes on top of an accumulator that
  /// already occupies 128 registers.
  template <typename FrgTensor, typename SmemA, typename SmemB>
  CUTLASS_DEVICE static void denoise_gemm(FrgTensor& tCrD, SmemA const& sA,
                                          SmemB const& sB, int thread_idx) {
    typename KTraits::TiledMmaDenoise tiled_mma;
    auto thr_mma = tiled_mma.get_thread_slice(thread_idx);

    auto s2r_copy_A =
        make_tiled_copy_A(typename KTraits::S2RCopyAtomDenoiseA{}, tiled_mma);
    auto s2r_thr_copy_A = s2r_copy_A.get_thread_slice(thread_idx);
    auto s2r_copy_B =
        make_tiled_copy_B(typename KTraits::S2RCopyAtomDenoiseB{}, tiled_mma);
    auto s2r_thr_copy_B = s2r_copy_B.get_thread_slice(thread_idx);

    constexpr int kAtomK = KTraits::kDenoiseAtomK;
    constexpr int k_blocks = R / kAtomK;

    // Deliberately not unrolled. Each iteration's B fragment is 64 registers,
    // so an eight-deep unroll keeps 512 of them live next to a 128-register
    // accumulator and spills ~780 bytes.
    CUTLASS_PRAGMA_NO_UNROLL
    for (int k_block = 0; k_block < k_blocks; ++k_block) {
      Tensor sA_k = local_tile(sA, Shape<Int<bM>, Int<kAtomK>>{},
                               make_coord(_0{}, k_block));
      Tensor sB_k = local_tile(sB, Shape<Int<bN>, Int<kAtomK>>{},
                               make_coord(_0{}, k_block));

      Tensor tCsA = s2r_thr_copy_A.partition_S(sA_k);
      Tensor tCsB = s2r_thr_copy_B.partition_S(sB_k);
      Tensor tCrA = thr_mma.partition_fragment_A(sA_k);
      Tensor tCrB = thr_mma.partition_fragment_B(sB_k);

      cute::copy(s2r_copy_A, tCsA, s2r_thr_copy_A.retile_D(tCrA));
      cute::copy(s2r_copy_B, tCsB, s2r_thr_copy_B.retile_D(tCrB));
      cute::gemm(tiled_mma, tCrA, tCrB, tCrD);
    }
  }

 public:
  /// Subtract both rank-R noise terms from the mainloop accumulator.
  ///
  /// Called with A and B no longer in use, since the factors are loaded over
  /// them.
  template <typename FrgTensor>
  CUTLASS_DEVICE void denoise(
      Params const& epilogue_params, FrgTensor& tCrD,
      SharedStorage& shared_storage,
      cute::tuple<int32_t, int32_t, int32_t> const& block_coord,
      int thread_idx) {

    auto [m_block, n_block, bidb] = block_coord;
    auto [M, N, K, R_] = epilogue_params.problem_shape;
    int const residual_M = M - m_block * bM;
    int const residual_N = N - n_block * bN;

    // The fp16 denoise factors were pre-scaled by 2^12 so the correction fits
    // fp16's range; the accumulator is brought into the same units, and taken
    // back out at the end.
    CUTLASS_PRAGMA_UNROLL
    for (int i = 0; i < size(tCrD); ++i) {
      tCrD(i) /= static_cast<float>(lattice::kIntToFp16ScaleFactor);
    }

    // ---- Phase 1: Y = -EAL x EARxBpEB ---------------------------------
    {
      Tensor sEAL = make_tensor(make_smem_ptr(shared_storage.smem_EAL.data()),
                                SmemLayoutEAL{});
      Tensor sEARxBpEB =
          make_tensor(make_smem_ptr(shared_storage.smem_EARxBpEB.data()),
                      SmemLayoutEARxBpEB{});
      load_denoise_factor(epilogue_params.EAL, epilogue_params.layout_M_denoise,
                          sEAL, m_block, residual_M, thread_idx);
      load_denoise_factor(epilogue_params.EARxBpEB,
                          epilogue_params.layout_N_denoise, sEARxBpEB, n_block,
                          residual_N, thread_idx);
      cute::cp_async_fence();
      cute::cp_async_wait<0>();
      __syncthreads();

      denoise_gemm(tCrD, sEAL, sEARxBpEB, thread_idx);
    }

    // Phase 2 is loaded over phase 1's operands, so everyone has to be done
    // reading them first.
    __syncthreads();

    // ---- Phase 2: X = -AxEBL x EBR ------------------------------------
    {
      Tensor sAxEBL =
          make_tensor(make_smem_ptr(shared_storage.smem_AxEBL.data()),
                      SmemLayoutAxEBL{});
      Tensor sEBR = make_tensor(make_smem_ptr(shared_storage.smem_EBR.data()),
                                SmemLayoutEBR{});
      load_denoise_factor(epilogue_params.AxEBL,
                          epilogue_params.layout_M_denoise, sAxEBL, m_block,
                          residual_M, thread_idx);
      load_denoise_factor(epilogue_params.EBR, epilogue_params.layout_N_denoise,
                          sEBR, n_block, residual_N, thread_idx);
      cute::cp_async_fence();
      cute::cp_async_wait<0>();
      __syncthreads();

      denoise_gemm(tCrD, sAxEBL, sEBR, thread_idx);
    }

    CUTLASS_PRAGMA_UNROLL
    for (int i = 0; i < size(tCrD); ++i) {
      tCrD(i) *= lattice::kIntToFp16ScaleFactor;
    }
  }

  /// Apply the per-row and per-column dequantisation scales and stage the tile
  /// in shared memory.
  template <typename FrgTensor, typename TiledMma>
  CUTLASS_DEVICE void scale(
      Params const& epilogue_params, FrgTensor& tCrD,
      SharedStorage& shared_storage, TiledMma tiled_mma, int thread_idx,
      cute::tuple<int32_t, int32_t, int32_t> const& block_coord) {

    auto [m_block, n_block, bidb] = block_coord;
    auto [M, N, K, R_] = epilogue_params.problem_shape;
    int const residual_M = M - m_block * bM;
    int const residual_N = N - n_block * bN;

    Tensor AScales = make_tensor(make_gmem_ptr(epilogue_params.ptr_A_scales),
                                 epilogue_params.layout_A_scales);
    Tensor BScales = make_tensor(make_gmem_ptr(epilogue_params.ptr_B_scales),
                                 epilogue_params.layout_B_scales);
    Tensor gAscales =
        local_tile(AScales, select<0>(TileShape_MNK{}), make_coord(m_block));
    Tensor gBscales =
        local_tile(BScales, select<1>(TileShape_MNK{}), make_coord(n_block));
    Tensor sAscales = make_tensor(
        make_smem_ptr(shared_storage.smem_scale_a.data()), SmemLayoutScaleA{});
    Tensor sBscales = make_tensor(
        make_smem_ptr(shared_storage.smem_scale_b.data()), SmemLayoutScaleB{});

    typename KTraits::G2SScalesCopyA g2s_scale_copy_a;
    typename KTraits::G2SScalesCopyB g2s_scale_copy_b;
    auto thr_a = g2s_scale_copy_a.get_slice(thread_idx);
    auto thr_b = g2s_scale_copy_b.get_slice(thread_idx);

    if (thread_idx < bM && (KTraits::Is_Even_M || thread_idx < residual_M)) {
      cute::copy(g2s_scale_copy_a, thr_a.partition_S(gAscales),
                 thr_a.partition_D(sAscales));
    }
    if (thread_idx < bN && (KTraits::Is_Even_N || thread_idx < residual_N)) {
      cute::copy(g2s_scale_copy_b, thr_b.partition_S(gBscales),
                 thr_b.partition_D(sBscales));
    }
    cute::cp_async_fence();
    cute::cp_async_wait<0>();
    // Doing double duty: it publishes the scales, and it is also what makes the
    // staged tile safe to write, since that lands on top of the denoise factors
    // and every thread has left the denoise GEMM by the time this releases.
    __syncthreads();

    Tensor sC =
        make_tensor(make_smem_ptr(shared_storage.smem_C.data()), SmemLayoutC{});

    // Scale, convert and stage in one pass over the accumulator. Converting the
    // whole fragment first, as the Hopper epilogue does before its stmatrix,
    // means holding 128 floats and 128 bfloat16s at once -- 64 registers this
    // kernel does not have to spare.
    auto thr_mma = tiled_mma.get_slice(thread_idx);
    Tensor cD = make_identity_tensor(select<0, 1>(TileShape_MNK{}));
    Tensor tCcD = thr_mma.partition_C(cD);
    CUTLASS_PRAGMA_UNROLL
    for (int j = 0; j < size<2>(tCrD); ++j) {
      CUTLASS_PRAGMA_UNROLL
      for (int i = 0; i < size<1>(tCrD); ++i) {
        CUTLASS_PRAGMA_UNROLL
        for (int v = 0; v < size<0>(tCrD); ++v) {
          int const m_idx = get<0>(tCcD(v, i, j));
          int const n_idx = get<1>(tCcD(v, i, j));
          float const scaled =
              tCrD(v, i, j) * (sAscales(m_idx) * sBscales(n_idx));
          sC(m_idx, n_idx) = static_cast<ElementOutput>(scaled);
        }
      }
    }
  }

  /// Write the staged tile out, replacing the TMA store with predicated vector
  /// stores.
  CUTLASS_DEVICE void store(
      Params const& epilogue_params, SharedStorage& shared_storage,
      int thread_idx,
      cute::tuple<int32_t, int32_t, int32_t> const& block_coord) {

    auto [m_block, n_block, bidb] = block_coord;
    auto [M, N, K, R_] = epilogue_params.problem_shape;
    int const residual_M = M - m_block * bM;
    int const residual_N = N - n_block * bN;

    __syncthreads();  // the staged tile must be complete before it is read back

    Tensor sC =
        make_tensor(make_smem_ptr(shared_storage.smem_C.data()), SmemLayoutC{});
    Tensor mC = make_tensor(make_gmem_ptr(epilogue_params.ptr_C),
                            epilogue_params.layout_C);
    Tensor gC = local_tile(mC, select<0, 1>(TileShape_MNK{}),
                           make_coord(m_block, n_block));

    typename KTraits::S2GTiledCopyC s2g_copy;
    auto thr_copy = s2g_copy.get_thread_slice(thread_idx);
    Tensor tCsC = thr_copy.partition_S(sC);
    Tensor tCgC = thr_copy.partition_D(gC);

    Tensor cC = make_identity_tensor(select<0, 1>(TileShape_MNK{}));
    Tensor tCcC = thr_copy.partition_D(cC);

    CUTLASS_PRAGMA_UNROLL
    for (int n = 0; n < size<2>(tCgC); ++n) {
      CUTLASS_PRAGMA_UNROLL
      for (int m = 0; m < size<1>(tCgC); ++m) {
        bool const in_bounds =
            (KTraits::Is_Even_M || get<0>(tCcC(_0{}, m, n)) < residual_M) &&
            (KTraits::Is_Even_N || get<1>(tCcC(_0{}, m, n)) < residual_N);
        if (in_bounds) {
          cute::copy(s2g_copy, tCsC(_, m, n), tCgC(_, m, n));
        }
      }
    }
  }

  CUTLASS_DEVICE void store_tail() {}
};

}  // namespace lattice::ada

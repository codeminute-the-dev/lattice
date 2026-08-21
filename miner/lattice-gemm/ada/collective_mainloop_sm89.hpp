#pragma once

// Ada (sm_89) counterpart of ../csrc/gemm/collective_mainloop.hpp.
//
// The Hopper mainloop is split in two: a producer warp issues TMA loads against
// an mbarrier pipeline while the consumer warpgroups run warpgroup MMA out of
// shared memory. Neither half survives the move to Ada, and the replacement is
// not a translation of each piece but the ordinary SM80 multistage shape --
// every thread issues its own cp.async, waits on commit groups, and runs
// synchronous MMA out of registers. So load and MMA are one function here
// rather than two.
//
// What does not change is the part consensus can see. The MMA atom's K extent
// is still 32, the accumulator's thread layout is the one Hopper produces, and
// the transcript is folded by the same TileHashAccumulator from
// ../csrc/gemm/pow_utils.hpp -- reused rather than reimplemented, because a
// second copy of that arithmetic is a second thing that can silently diverge.

#include <cutlass/array.h>
#include <cutlass/cutlass.h>
#include <cutlass/numeric_types.h>

#include "cute/tensor.hpp"

#include "blake3/blake3_constants.hpp"
#include "gemm/host_signal_header.hpp"
#include "gemm/pow_utils.hpp"

#include "kernel_traits_sm89.hpp"

namespace lattice::ada {

using namespace cute;

template <typename KTraits>
struct CollectiveMainloopSm89 {

  using ElementIn = typename KTraits::ElementIn;
  using TileShape_MNK = typename KTraits::TileShape_MNK;
  using TileShape_MNR = typename KTraits::TileShape_MNR;
  using ProblemShape = typename KTraits::ProblemShape;

  using SmemLayoutA = typename KTraits::SmemLayoutA;
  using SmemLayoutB = typename KTraits::SmemLayoutB;

  using ShapeT = cute::Shape<int32_t, int32_t>;
  using StrideT = cute::Stride<int32_t, _1>;
  using LayoutT = cute::Layout<ShapeT, StrideT>;

  using MMAAtom_K = typename KTraits::MMAAtom_K;

  static constexpr int kStages = KTraits::kStages;
  static constexpr bool SkipReduction = KTraits::SkipReduction;
  static constexpr int bM = KTraits::bM;
  static constexpr int bN = KTraits::bN;
  static constexpr int bK = KTraits::bK;
  static_assert(kStages >= 2, "the multistage loop needs at least two stages");

  struct Arguments {
    ElementIn const* ptr_A;
    ElementIn const* ptr_B;
    void* host_signal_header_pinned;
    void* host_signal_sync;
    ProblemShape const problem_shape;
    uint64_t* inner_hash_counter;
    uint32_t const* ptr_pow_target;
    uint32_t const* ptr_pow_key;
  };

  struct Params {
    ElementIn const* ptr_A;
    ElementIn const* ptr_B;
    LayoutT layout_A;
    LayoutT layout_B;
    HostSignalHeader* host_signal_header_pinned;
    HostSignalSync* host_signal_sync;
    ProblemShape const problem_shape;
    uint64_t* inner_hash_counter;
    uint32_t const* ptr_pow_target;
    uint32_t const* ptr_pow_key;
  };

  static Params to_underlying_arguments(Arguments const& args) {
    auto [M, N, K, R] = args.problem_shape;
    LayoutT layout_A = make_layout(make_shape(M, K), make_stride(K, _1{}));
    LayoutT layout_B = make_layout(make_shape(N, K), make_stride(K, _1{}));
    return {.ptr_A = args.ptr_A,
            .ptr_B = args.ptr_B,
            .layout_A = layout_A,
            .layout_B = layout_B,
            .host_signal_header_pinned = reinterpret_cast<HostSignalHeader*>(
                args.host_signal_header_pinned),
            .host_signal_sync =
                reinterpret_cast<HostSignalSync*>(args.host_signal_sync),
            .problem_shape = args.problem_shape,
            .inner_hash_counter = args.inner_hash_counter,
            .ptr_pow_target = args.ptr_pow_target,
            .ptr_pow_key = args.ptr_pow_key};
  }

  /// Fill a predicate over a thread's (CPY_M, CPY_K) slice of one k-tile.
  ///
  /// TMA takes bounds from its descriptor and zero-fills whatever falls outside
  /// the tensor. cp.async has to be told, so every remainder in M, N and K is
  /// handled here; the ZFILL atom then writes zeros where the predicate is
  /// false, which is what makes a partial k-tile contribute nothing to the
  /// accumulator.
  template <typename PredTensor, typename CoordTensor>
  CUTLASS_DEVICE static void fill_predicate(PredTensor& pred,
                                            CoordTensor const& coord,
                                            int residual_mn, int k_offset,
                                            int K) {
    CUTLASS_PRAGMA_UNROLL
    for (int k = 0; k < size<2>(coord); ++k) {
      CUTLASS_PRAGMA_UNROLL
      for (int m = 0; m < size<1>(coord); ++m) {
        bool const in_mn = get<0>(coord(_0{}, m, k)) < residual_mn;
        bool const in_k = k_offset + get<1>(coord(_0{}, m, k)) < K;
        pred(m, k) = in_mn && in_k;
      }
    }
  }

  /// Mainloop: stage A and B through shared memory and accumulate A x B^T into
  /// tCrC, folding the transcript as it goes.
  ///
  /// Every thread both loads and multiplies, so this is called by the whole CTA.
  template <typename SharedStorage, typename FrgTensorC,
            typename TranscriptTensor>
  CUTLASS_DEVICE void mma(Params const& mainloop_params,
                          SharedStorage& shared_storage,
                          cute::tuple<int32_t, int32_t, int32_t> block_coord,
                          int k_tile_count, FrgTensorC& tCrC,
                          TranscriptTensor& transcript_extraction_tensor,
                          int thread_idx) {

    auto [m_block, n_block, bidb] = block_coord;
    auto [M, N, K, R] = mainloop_params.problem_shape;
    int const residual_M = M - m_block * bM;
    int const residual_N = N - n_block * bN;

    Tensor sA = make_tensor(make_smem_ptr(shared_storage.smem_A.data()),
                            SmemLayoutA{});  // (bM, bK, PIPE)
    Tensor sB = make_tensor(make_smem_ptr(shared_storage.smem_B.data()),
                            SmemLayoutB{});  // (bN, bK, PIPE)

    Tensor mA = make_tensor(make_gmem_ptr(mainloop_params.ptr_A),
                            mainloop_params.layout_A);
    Tensor mB = make_tensor(make_gmem_ptr(mainloop_params.ptr_B),
                            mainloop_params.layout_B);

    Tensor gA = local_tile(mA, select<0, 2>(TileShape_MNK{}),
                           make_coord(m_block, _));  // (bM, bK, k)
    Tensor gB = local_tile(mB, select<1, 2>(TileShape_MNK{}),
                           make_coord(n_block, _));  // (bN, bK, k)

    // ---- GMEM -> SMEM -------------------------------------------------
    typename KTraits::G2STiledCopyIn g2s_copy;
    auto g2s_thr_copy = g2s_copy.get_thread_slice(thread_idx);

    Tensor tAgA = g2s_thr_copy.partition_S(gA);  // (CPY, CPY_M, CPY_K, k)
    Tensor tAsA = g2s_thr_copy.partition_D(sA);  // (CPY, CPY_M, CPY_K, PIPE)
    Tensor tBgB = g2s_thr_copy.partition_S(gB);
    Tensor tBsB = g2s_thr_copy.partition_D(sB);

    Tensor cA = make_identity_tensor(select<0, 2>(TileShape_MNK{}));
    Tensor cB = make_identity_tensor(select<1, 2>(TileShape_MNK{}));
    Tensor tAcA = g2s_thr_copy.partition_S(cA);
    Tensor tBcB = g2s_thr_copy.partition_S(cB);

    Tensor tApA = make_tensor<bool>(
        make_shape(size<1>(tAsA), size<2>(tAsA)));
    Tensor tBpB = make_tensor<bool>(
        make_shape(size<1>(tBsB), size<2>(tBsB)));

    auto load_k_tile = [&](int k_tile, int stage) {
      if (k_tile < k_tile_count) {
        int const k_offset = k_tile * bK;
        fill_predicate(tApA, tAcA, residual_M, k_offset, K);
        fill_predicate(tBpB, tBcB, residual_N, k_offset, K);
        cute::copy_if(g2s_copy, tApA, tAgA(_, _, _, k_tile),
                      tAsA(_, _, _, stage));
        cute::copy_if(g2s_copy, tBpB, tBgB(_, _, _, k_tile),
                      tBsB(_, _, _, stage));
      }
      // Commit unconditionally: the wait counts groups, so a skipped tail tile
      // still has to contribute one.
      cute::cp_async_fence();
    };

    // ---- SMEM -> RMEM -------------------------------------------------
    typename KTraits::TiledMma tiled_mma;
    auto thr_mma = tiled_mma.get_thread_slice(thread_idx);

    auto s2r_copy_A =
        make_tiled_copy_A(typename KTraits::S2RCopyAtomA{}, tiled_mma);
    auto s2r_thr_copy_A = s2r_copy_A.get_thread_slice(thread_idx);
    auto s2r_copy_B =
        make_tiled_copy_B(typename KTraits::S2RCopyAtomB{}, tiled_mma);
    auto s2r_thr_copy_B = s2r_copy_B.get_thread_slice(thread_idx);

    Tensor tCsA = s2r_thr_copy_A.partition_S(sA);  // (CPY, CPY_M, CPY_K, PIPE)
    Tensor tCsB = s2r_thr_copy_B.partition_S(sB);

    Tensor tCrA = thr_mma.partition_fragment_A(sA(_, _, _0{}));  // (MMA,M,K)
    Tensor tCrB = thr_mma.partition_fragment_B(sB(_, _, _0{}));  // (MMA,N,K)
    Tensor tCrA_view = s2r_thr_copy_A.retile_D(tCrA);
    Tensor tCrB_view = s2r_thr_copy_B.retile_D(tCrB);

    // ---- Transcript ---------------------------------------------------
    const uint32_t last_full_k_block =
        shape<1>(mainloop_params.layout_A) / MMAAtom_K{};
    constexpr int k_blocks_per_tile = size<2>(tCrA);
    constexpr int reduce_every_k = get<2>(TileShape_MNR{}) / MMAAtom_K{};

    using HashAccumulator =
        lattice::TileHashAccumulator<k_blocks_per_tile, reduce_every_k,
                                     KTraits::EnableDebug>;
    HashAccumulator hash_accumulator(last_full_k_block,
                                     mainloop_params.inner_hash_counter);

    // ---- Prologue: prime the pipeline ---------------------------------
    CUTLASS_PRAGMA_UNROLL
    for (int stage = 0; stage < kStages - 1; ++stage) {
      load_k_tile(stage, stage);
    }

    // ---- Main loop ----------------------------------------------------
    CUTLASS_PRAGMA_NO_UNROLL
    for (int k_tile = 0; k_tile < k_tile_count; ++k_tile) {
      // Leave kStages-2 groups in flight, which is exactly the condition that
      // this tile's stage has landed.
      cute::cp_async_wait<kStages - 2>();
      __syncthreads();

      int const stage = k_tile % kStages;

      // Safe to overwrite now: the stage being refilled is the one read last
      // iteration, and the barrier above means every thread is done with it.
      load_k_tile(k_tile + kStages - 1, (k_tile + kStages - 1) % kStages);

      if constexpr (!SkipReduction) {
        hash_accumulator.preload(transcript_extraction_tensor);
      }

      CUTLASS_PRAGMA_UNROLL
      for (int k_block = 0; k_block < k_blocks_per_tile; ++k_block) {
        cute::copy(s2r_copy_A, tCsA(_, _, k_block, stage),
                   tCrA_view(_, _, k_block));
        cute::copy(s2r_copy_B, tCsB(_, _, k_block, stage),
                   tCrB_view(_, _, k_block));
        cute::gemm(tiled_mma, tCrA(_, _, k_block), tCrB(_, _, k_block), tCrC);

        if constexpr (!SkipReduction) {
          hash_accumulator.accumulate(tCrC, k_block);
        }
      }

      if constexpr (!SkipReduction) {
        hash_accumulator.writeback(transcript_extraction_tensor);
      }
    }

    // The epilogue reuses this shared memory for the denoise factors and the
    // output tile, so nobody may still be reading A or B when it starts.
    cute::cp_async_wait<0>();
    __syncthreads();
  }
};

}  // namespace lattice::ada

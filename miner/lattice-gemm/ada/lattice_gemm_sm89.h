#pragma once

// Ada (sm_89) NoisyGEMM: the kernel, and the launch that configures it.
//
// Counterpart of ../csrc/gemm/lattice_gemm_kernel.h and lattice_gemm_host.h.
// The Hopper kernel spends most of its length on warp specialisation --
// splitting the CTA into a producer warpgroup and consumer warpgroups, sizing
// their register budgets, and wiring three mbarrier pipelines between them.
// None of that has an Ada equivalent, and none of it is needed: with cp.async
// every thread loads for itself, so the kernel is a straight line through the
// mainloop and the epilogue.
//
// The launch is a plain triple-chevron rather than a cluster launch, and it
// opts in to the full 100 KB of shared memory, which the tile requires.

#include <cute/tensor.hpp>

#include "blake3/blake3_constants.hpp"
#include "gemm/host_signal_header.hpp"
#include "gemm/pow_utils.hpp"

#include "collective_epilogue_sm89.hpp"
#include "collective_mainloop_sm89.hpp"
#include "kernel_traits_sm89.hpp"
#include "tile_scheduler_sm89.hpp"

namespace lattice::ada {

using namespace cute;

template <typename KTraits, typename TileScheduler>
__global__ void __launch_bounds__(KTraits::kNumThreads, 1)
    ada_noisy_gemm(CUTE_GRID_CONSTANT typename CollectiveMainloopSm89<
                       KTraits>::Params const mainloop_params,
                   CUTE_GRID_CONSTANT typename CollectiveEpilogueSm89<
                       KTraits>::Params const epilogue_params,
                   CUTE_GRID_CONSTANT
                   typename TileScheduler::Params const scheduler_params) {

  using TileShape_MNK = typename KTraits::TileShape_MNK;
  using CollectiveMainloop = CollectiveMainloopSm89<KTraits>;
  using CollectiveEpilogue = CollectiveEpilogueSm89<KTraits>;
  static constexpr bool SkipDenoising = KTraits::SkipDenoising;
  static constexpr bool SkipReduction = KTraits::SkipReduction;

  extern __shared__ char shared_memory[];
  auto& shared_storage =
      *reinterpret_cast<typename KTraits::SharedStorage*>(shared_memory);

  int const thread_idx = threadIdx.x;
  int const k_tile_count =
      cutlass::ceil_div(shape<1>(mainloop_params.layout_A), KTraits::bK);

  CollectiveMainloop collective_mainloop;
  CollectiveEpilogue collective_epilogue;
  TileScheduler scheduler;

  typename KTraits::TiledMma tiled_mma;

  auto work_tile_info = scheduler.get_initial_work(scheduler_params);
  CUTLASS_PRAGMA_NO_UNROLL
  while (work_tile_info.is_valid(scheduler_params)) {
    cute::tuple<int32_t, int32_t, int32_t> block_coord =
        work_tile_info.get_block_coord(scheduler_params);

    Tensor tCrC =
        partition_fragment_C(tiled_mma, select<0, 1>(TileShape_MNK{}));
    clear(tCrC);

    auto transcript_extraction_tensor =
        make_tensor<uint32_t>(Int<blake3::MSG_BLOCK_SIZE_U32>{});
    if constexpr (!SkipReduction) {
      clear(transcript_extraction_tensor);
    }

    collective_mainloop.mma(mainloop_params, shared_storage, block_coord,
                            k_tile_count, tCrC, transcript_extraction_tensor,
                            thread_idx);

    Tensor tCrD_fp32 = make_tensor_like<float>(tCrC);
    CUTLASS_PRAGMA_UNROLL
    for (int i = 0; i < size(tCrD_fp32); ++i) {
      tCrD_fp32(i) = static_cast<float>(tCrC(i));
    }

    if constexpr (!SkipDenoising) {
      collective_epilogue.denoise(epilogue_params, tCrD_fp32, shared_storage,
                                  block_coord, thread_idx);
    }

    collective_epilogue.scale(epilogue_params, tCrD_fp32, shared_storage,
                              tiled_mma, thread_idx, block_coord);
    collective_epilogue.store(epilogue_params, shared_storage, thread_idx,
                              block_coord);

    if constexpr (!SkipReduction) {
      bool const local_block_found =
          lattice::check_pow_target(transcript_extraction_tensor,
                                    mainloop_params.ptr_pow_target,
                                    mainloop_params.ptr_pow_key);
      if (local_block_found) {
        lattice::write_host_signal_header<typename KTraits::TiledMma,
                                          TileShape_MNK>(
            mainloop_params.host_signal_sync,
            mainloop_params.host_signal_header_pinned,
            mainloop_params.problem_shape, block_coord, thread_idx,
            mainloop_params.ptr_pow_target);
      }
    }

    collective_epilogue.store_tail();
    work_tile_info = scheduler.get_next_work(scheduler_params, work_tile_info);
  }
}

/// Everything the kernel needs from the caller. Mirrors LatticeAPIParams
/// (../csrc/gemm/lattice_api_params.h) but without the torch dependency, so a
/// standalone test can drive the kernel.
struct AdaGemmParams {
  void const* ptr_ApEA;
  void const* ptr_BpEB;
  void* ptr_C;
  void const* ptr_A_scales;
  void const* ptr_B_scales;
  void const* ptr_EAL_mma;
  void const* ptr_EARxBpEB_mma;
  void const* ptr_AxEBL_mma;
  void const* ptr_EBR_mma;
  void* host_signal_header_pinned;
  void* host_signal_sync;
  uint64_t* inner_hash_counter;
  void const* ptr_pow_target;
  void const* ptr_pow_key;
  int m, n, k, r;
  int swizzle;
  bool swizzle_n_maj;
};

template <class ElementOut_, typename TileShape_MNKR, int KStages_,
          bool Is_Even_M = true, bool Is_Even_N = true,
          bool SkipReduction = false, bool SkipDenoising = false,
          bool EnableDebug = false>
cudaError_t run_lattice_gemm_sm89(AdaGemmParams const& params,
                                  cudaStream_t stream = 0) {
  using ElementIn = int8_t;
  using ElementDenoise = cutlass::half_t;
  using ElementScale = float;
  using ElementOut = ElementOut_;

  using KTraits =
      KernelTraitsSm89<ElementIn, ElementOut, ElementDenoise, ElementScale,
                       TileShape_MNKR, Is_Even_M, Is_Even_N, SkipReduction,
                       SkipDenoising, KStages_, EnableDebug>;
  using Mainloop = CollectiveMainloopSm89<KTraits>;
  using Epilogue = CollectiveEpilogueSm89<KTraits>;
  using Scheduler = SingleTileSchedulerSm89;

  auto problem_shape = make_shape(params.m, params.n, params.k, params.r);

  typename Mainloop::Arguments mainloop_args{
      .ptr_A = static_cast<ElementIn const*>(params.ptr_ApEA),
      .ptr_B = static_cast<ElementIn const*>(params.ptr_BpEB),
      .host_signal_header_pinned = params.host_signal_header_pinned,
      .host_signal_sync = params.host_signal_sync,
      .problem_shape = problem_shape,
      .inner_hash_counter = params.inner_hash_counter,
      .ptr_pow_target = static_cast<uint32_t const*>(params.ptr_pow_target),
      .ptr_pow_key = static_cast<uint32_t const*>(params.ptr_pow_key)};

  typename Epilogue::Arguments epilogue_args{
      .ptr_C = static_cast<ElementOut*>(params.ptr_C),
      .ptr_A_scales = static_cast<ElementScale const*>(params.ptr_A_scales),
      .ptr_B_scales = static_cast<ElementScale const*>(params.ptr_B_scales),
      .ptr_EAL = static_cast<ElementDenoise const*>(params.ptr_EAL_mma),
      .ptr_EARxBpEB =
          static_cast<ElementDenoise const*>(params.ptr_EARxBpEB_mma),
      .ptr_AxEBL = static_cast<ElementDenoise const*>(params.ptr_AxEBL_mma),
      .ptr_EBR = static_cast<ElementDenoise const*>(params.ptr_EBR_mma),
      .problem_shape = problem_shape};

  // TMA reads its bounds from a descriptor and copes with any stride. cp.async
  // and the vector store do not: a 16-byte access has to be 16-byte aligned, so
  // each row of A, B and C has to start on one. Row *counts* are free -- M and
  // N are predicated -- but the row pitches are not.
  if (params.k % KTraits::kGmemElemsPerLoadIn != 0 ||
      params.n % KTraits::kGmemElemsPerStoreOut != 0 ||
      params.r % KTraits::kGmemElemsPerLoadDenoise != 0) {
    return cudaErrorInvalidValue;
  }

  int const num_blocks_m = cutlass::ceil_div(params.m, KTraits::bM);
  int const num_blocks_n = cutlass::ceil_div(params.n, KTraits::bN);
  typename Scheduler::Arguments scheduler_args{
      .num_blocks_m = num_blocks_m,
      .num_blocks_n = num_blocks_n,
      .swizzle = params.swizzle > 0 ? params.swizzle : 1,
      .swizzle_n_maj = params.swizzle_n_maj};

  int device = 0;
  cudaGetDevice(&device);
  int multiprocessor_count = 0;
  cudaDeviceGetAttribute(&multiprocessor_count, cudaDevAttrMultiProcessorCount,
                         device);

  auto kernel = ada_noisy_gemm<KTraits, Scheduler>;
  int const smem_size = static_cast<int>(sizeof(typename KTraits::SharedStorage));
  if (smem_size >= 48 * 1024) {
    cudaError_t const attr = cudaFuncSetAttribute(
        kernel, cudaFuncAttributeMaxDynamicSharedMemorySize, smem_size);
    if (attr != cudaSuccess) {
      return attr;
    }
  }

  dim3 const grid = Scheduler::get_grid_dim(scheduler_args, multiprocessor_count);
  dim3 const block(KTraits::kNumThreads);

  kernel<<<grid, block, smem_size, stream>>>(
      Mainloop::to_underlying_arguments(mainloop_args),
      Epilogue::to_underlying_arguments(epilogue_args),
      Scheduler::to_underlying_arguments(scheduler_args));

  return cudaGetLastError();
}

}  // namespace lattice::ada

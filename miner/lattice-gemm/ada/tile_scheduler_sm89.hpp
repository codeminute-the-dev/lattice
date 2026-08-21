#pragma once

// Ada (sm_89) counterpart of SingleTileScheduler in ../csrc/gemm/tile_scheduler.hpp.
//
// The Hopper scheduler swizzles in units of thread-block clusters and reads its
// position with cluster_id_in_grid() / block_id_in_cluster(). Ada has no
// clusters, and those are sm_90 intrinsics, so the position comes from blockIdx
// and a cluster is exactly one CTA.
//
// The L2 swizzle itself is worth keeping and is unchanged: it is what stops
// concurrent CTAs from streaming disjoint columns of B, and that matters more
// on Ada, whose L2 is a fraction of a Hopper part's. The helpers it needs are
// already cluster-free, so they are reused rather than copied.

#include "cutlass/fast_math.h"

#include "gemm/tile_scheduler.hpp"

namespace lattice::ada {

struct SingleTileSchedulerSm89 {

  struct Arguments {
    int const num_blocks_m, num_blocks_n, swizzle;
    bool const swizzle_n_maj;
  };

  using Params = lattice::SwizzleParams;

  static Params to_underlying_arguments(Arguments const& args) {
    // One CTA per cluster, so the cluster counts are the block counts.
    lattice::SwizzleArgs swizzle_args{.num_clusters_m = args.num_blocks_m,
                                      .num_clusters_n = args.num_blocks_n,
                                      .swizzle = args.swizzle,
                                      .swizzle_n_maj = args.swizzle_n_maj};
    return lattice::make_swizzle_params(swizzle_args);
  }

  static dim3 get_grid_dim(Arguments const& args, int num_sm) {
    return {uint32_t(args.num_blocks_m), uint32_t(args.num_blocks_n), 1};
  }

  struct WorkTileInfo {
    int linear_idx;
    bool is_valid_tile = false;

    CUTLASS_DEVICE
    bool is_valid(Params const& params) const { return is_valid_tile; }

    CUTLASS_DEVICE cute::tuple<int32_t, int32_t, int32_t> get_block_coord(
        Params const& params) const {
      auto [nonmaj, maj] =
          lattice::get_coords_from_linear_idx(params, linear_idx);
      int const m_block = params.swizzle_n_maj ? nonmaj : maj;
      int const n_block = params.swizzle_n_maj ? maj : nonmaj;
      return {m_block, n_block, 1};
    }
  };

  CUTLASS_DEVICE
  SingleTileSchedulerSm89() {}

  CUTLASS_DEVICE
  WorkTileInfo get_initial_work(Params const& params) const {
    return {int(blockIdx.x) + int(blockIdx.y) * params.num_clusters_m, true};
  }

  CUTLASS_DEVICE
  WorkTileInfo get_next_work(Params const& params,
                             WorkTileInfo const& current_work) const {
    return {-1, false};
  }
};

}  // namespace lattice::ada

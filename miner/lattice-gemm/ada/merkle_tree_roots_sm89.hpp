#pragma once

// Ada (sm_89) counterpart of ../csrc/tensor_hash/merkle_tree_roots_kernel.hpp.
//
// One thread hashes one 1 KiB BLAKE3 chunk, so a warp's 32 threads want 32
// chunks 1 KiB apart -- an access pattern that reads terribly straight from
// global memory. The Hopper kernel fixes that by staging through shared memory
// with TMA, which costs it a producer warpgroup and a pipeline.
//
// cp.async solves the same problem without either: the threads cooperatively
// load a contiguous slab covering everybody's current slice and land it in the
// same shared-memory layout, so every thread still reads its own chunk out of
// shared memory. There is nothing left for a producer warpgroup to do.
//
// That also removes the dual-pipeline mode. It exists only because a TMA
// descriptor dimension cannot exceed 256, so 512 consumers needed two
// descriptors, two pipelines and a second copy of the consumer loop. cp.async
// has no such limit and 512 consumers are one loop again.
//
// The hashing itself is untouched: the same blake3 compression, the same
// per-chunk flags, and merkle_tree_utils for the reduction.

#include "blake3/blake3.cuh"
#include "cute/tensor.hpp"
#include "tensor_hash/merkle_tree_utils.hpp"
#include "tensor_hash/tensor_hash_constants.cuh"

#include <cutlass/cutlass.h>
#include <cutlass/gemm/collective/builders/sm90_common.inl>

namespace lattice::ada {

using namespace cute;

/// kNumThreads: threads per CTA, one per chunk.
/// kNumStages:  cp.async pipeline depth.
/// kThreadLoadSize: bytes of a chunk staged per iteration.
/// kApplyRoot: whether this kernel applies BLAKE3's ROOT finalisation, which
///   the host sets only when this is the single CTA over the whole input.
template <int kNumThreads_, int kNumStages_, int kThreadLoadSize_,
          bool kApplyRoot_>
struct MerkleTreeRootsKernelSm89 {

  using Element = uint8_t;
  using u32 = uint32_t;

  static constexpr int kNumThreads = kNumThreads_;
  static constexpr int kNumStages = kNumStages_;
  static constexpr int kThreadLoadSize = kThreadLoadSize_;
  static constexpr bool kApplyRoot = kApplyRoot_;

  static constexpr int kChunkSize = 1024;
  static constexpr int kWordSize = 4;
  static constexpr int kWordsPerChunk = kChunkSize / kWordSize;
  static constexpr int kNumWordsPerLoad = kThreadLoadSize / kWordSize;
  static constexpr int kNumLoads = kChunkSize / kThreadLoadSize;
  static constexpr int kNumWordsPerBlock =
      blake3::MSG_BLOCK_SIZE / sizeof(uint32_t);
  static constexpr int kNumBlocksPerLoad =
      kThreadLoadSize / blake3::MSG_BLOCK_SIZE;
  static constexpr int kNumBlocksPerChunk = kChunkSize / blake3::MSG_BLOCK_SIZE;

  static_assert(kThreadLoadSize == 64 || kThreadLoadSize == 128 ||
                    kThreadLoadSize == 256 || kThreadLoadSize == 512,
                "kThreadLoadSize must be 64, 128, 256, or 512");
  static_assert(kChunkSize % kThreadLoadSize == 0);
  static_assert(kNumStages >= 2);

  // ---------------------------------------------------------------------
  // Shared memory
  // ---------------------------------------------------------------------

  using SmemLayoutAtomA =
      std::conditional_t<kThreadLoadSize == 64,
                         GMMA::Layout_K_SW64_Atom<uint32_t>,
                         GMMA::Layout_K_SW128_Atom<uint32_t>>;
  using SmemLayoutA = decltype(tile_to_shape(
      SmemLayoutAtomA{}, make_shape(Int<kNumThreads>{}, Int<kNumWordsPerLoad>{},
                                    Int<kNumStages>{})));
  using SmemLayoutAtomLeaves = GMMA::Layout_K_SW128_Atom<uint32_t>;
  using SmemLayoutLeaves = decltype(tile_to_shape(
      SmemLayoutAtomLeaves{},
      Shape<Int<blake3::CHAINING_VALUE_SIZE_U32>, Int<kNumThreads>>{}));

  static constexpr size_t AlignmentA =
      cutlass::detail::alignment_for_swizzle(SmemLayoutA{});
  static constexpr size_t AlignmentLeaves =
      cutlass::detail::alignment_for_swizzle(SmemLayoutLeaves{});
  static constexpr size_t Alignment = cute::max(AlignmentA, AlignmentLeaves);

  struct SharedStorage : cute::aligned_struct<Alignment> {
    cute::array_aligned<uint32_t, cute::cosize_v<SmemLayoutLeaves>,
                        AlignmentLeaves>
        smem_leaves;
    cute::array_aligned<uint32_t, cute::cosize_v<SmemLayoutA>, AlignmentA>
        smem_a;
  };

  static constexpr int SharedStorageSize = sizeof(SharedStorage);
  static_assert(SharedStorageSize <= 101376,
                "staged chunk slices exceed Ada's shared memory; reduce "
                "kThreadLoadSize or kNumStages");

  // ---------------------------------------------------------------------
  // Loads
  // ---------------------------------------------------------------------

  static constexpr int kElemsPerLoad = 4;  // 16 B of uint32
  static constexpr int kThreadsPerRow = kNumWordsPerLoad / kElemsPerLoad;
  static_assert(kThreadsPerRow >= 1);
  static_assert(kNumThreads % kThreadsPerRow == 0);

  using G2SCopy = decltype(make_tiled_copy(
      Copy_Atom<SM80_CP_ASYNC_CACHEGLOBAL_ZFILL<uint128_t>, uint32_t>{},
      Layout<Shape<Int<kNumThreads / kThreadsPerRow>, Int<kThreadsPerRow>>,
             Stride<Int<kThreadsPerRow>, _1>>{},
      Layout<Shape<_1, Int<kElemsPerLoad>>>{}));

  // ---------------------------------------------------------------------

  struct Arguments {
    Element const* ptr_data;
    u32 data_len;  // bytes
    Element* ptr_roots;
  };

  struct alignas(128) Params {
    Element const* ptr_data;
    u32 data_len;
    Element* ptr_roots;
  };

  static Params to_underlying_arguments(Arguments const& args) {
    return {args.ptr_data, args.data_len, args.ptr_roots};
  }

  /// Chunks the input spans, counting a trailing partial one.
  CUTLASS_HOST_DEVICE static u32 compute_num_chunks(u32 data_len) {
    return (data_len + kChunkSize - 1) / kChunkSize;
  }

  /// Chunks fully backed by input bytes. The staged loads cover exactly these;
  /// a trailing partial chunk is read straight from global memory instead.
  CUTLASS_HOST_DEVICE static u32 compute_num_full_chunks(u32 data_len) {
    return data_len / kChunkSize;
  }

  static dim3 get_grid_shape(Params const& params) {
    u32 const num_chunks = compute_num_chunks(params.data_len);
    return dim3((num_chunks + kNumThreads - 1) / kNumThreads);
  }

  static dim3 get_block_shape() { return dim3(kNumThreads); }
};

/// Read one slice of the trailing partial chunk straight from global memory,
/// zero-filling past the end of the data. At most 1 KiB per hash is read this
/// way, by the single thread that owns that chunk.
template <class Kernel, class SmemTensorA>
CUTLASS_DEVICE void load_partial_chunk_sm89(
    typename Kernel::Params const& params, SmemTensorA& sA, int smem_row,
    int stage, int load_idx, uint32_t last_chunk_len) {
  uint32_t const chunk_start_byte =
      (params.data_len / Kernel::kChunkSize) * Kernel::kChunkSize;
  uint32_t const load_start_byte = load_idx * Kernel::kThreadLoadSize;
  auto const* chunk_ptr = params.ptr_data + chunk_start_byte;

  CUTLASS_PRAGMA_UNROLL
  for (int w = 0; w < Kernel::kNumWordsPerLoad; ++w) {
    uint32_t const word_start_byte = load_start_byte + w * sizeof(uint32_t);
    uint32_t word = 0;
    if (word_start_byte < last_chunk_len) {
      uint32_t const remaining = last_chunk_len - word_start_byte;
      auto const* src = chunk_ptr + word_start_byte;
      if (remaining >= sizeof(uint32_t)) {
        word = *reinterpret_cast<uint32_t const*>(src);
      } else {
        for (uint32_t b = 0; b < remaining; ++b) {
          word |= uint32_t(src[b]) << (8 * b);
        }
      }
    }
    sA(smem_row, w, stage) = word;
  }
}

/// Compress one 64-byte block of this thread's chunk into its chaining value.
template <class Kernel, bool IsSingleChunk, class SmemTensorA,
          class RmemTensorChainingValue>
CUTLASS_DEVICE void compress_block_sm89(SmemTensorA const& sA,
                                        RmemTensorChainingValue& cv, int tid,
                                        int stage, int block_in_load,
                                        int block_idx) {
  Tensor rBlock = make_tensor<uint32_t>(
      Layout<Shape<Int<Kernel::kNumWordsPerBlock>>>{});
  int const word_offset = block_in_load * Kernel::kNumWordsPerBlock;

  CUTLASS_PRAGMA_UNROLL
  for (int i = 0; i < Kernel::kNumWordsPerBlock / 4; ++i) {
    uint4 tmp = *reinterpret_cast<uint4 const*>(
        &sA(tid, word_offset + i * 4, stage));
    rBlock(i * 4 + 0) = tmp.x;
    rBlock(i * 4 + 1) = tmp.y;
    rBlock(i * 4 + 2) = tmp.z;
    rBlock(i * 4 + 3) = tmp.w;
  }

  blake3::CompressParams cp{
      .counter = blockIdx.x * Kernel::kNumThreads + tid,
      .block_len = blake3::MSG_BLOCK_SIZE,
      .flags = blake3::KEYED_HASH};
  if (block_idx == 0) {
    cp.flags |= blake3::CHUNK_START;
  }
  if (block_idx == Kernel::kNumBlocksPerChunk - 1) {
    cp.flags |= blake3::CHUNK_END;
    // A single-chunk message has no Merkle parent to finalise, so this last
    // compression is the one that has to carry ROOT.
    if constexpr (IsSingleChunk) {
      cp.flags |= blake3::ROOT;
    }
  }

  blake3::compress_msg_block_u32(rBlock, cv, cp);
}

template <class Kernel>
__global__ void __launch_bounds__(Kernel::kNumThreads, 1)
    ada_merkle_tree_roots(CUTE_GRID_CONSTANT
                          typename Kernel::Params const params) {

  using u32 = uint32_t;
  constexpr int kNumThreads = Kernel::kNumThreads;
  constexpr int kNumStages = Kernel::kNumStages;
  constexpr int kNumWordsPerLoad = Kernel::kNumWordsPerLoad;
  constexpr int kNumLoads = Kernel::kNumLoads;

  extern __shared__ char smem_buf[];
  auto& ss = *reinterpret_cast<typename Kernel::SharedStorage*>(smem_buf);

  int const tid = threadIdx.x;

  Tensor sA = as_position_independent_swizzle_tensor(make_tensor(
      make_smem_ptr(ss.smem_a.data()), typename Kernel::SmemLayoutA{}));
  Tensor sLeaves = as_position_independent_swizzle_tensor(make_tensor(
      make_smem_ptr(ss.smem_leaves.data()), typename Kernel::SmemLayoutLeaves{}));

  u32 const num_chunks = Kernel::compute_num_chunks(params.data_len);
  u32 const num_full_chunks = Kernel::compute_num_full_chunks(params.data_len);
  size_t const num_grid_blocks = (num_chunks + kNumThreads - 1) / kNumThreads;
  bool const is_single_chunk = (num_chunks == 1);

  // Rows past the last full chunk are zero-filled by the predicate, so the
  // pointer is never dereferenced even when there is no full chunk at all --
  // but the layout still needs a non-zero extent.
  Tensor mA = make_tensor(
      make_gmem_ptr(reinterpret_cast<uint32_t const*>(params.ptr_data)),
      make_shape(int(cute::max(num_full_chunks, 1u)), Int<Kernel::kWordsPerChunk>{}),
      make_stride(Int<Kernel::kWordsPerChunk>{}, Int<1>{}));
  Tensor gA = local_tile(mA, Shape<Int<kNumThreads>, Int<kNumWordsPerLoad>>{},
                         make_coord(int(blockIdx.x), _));

  typename Kernel::G2SCopy g2s;
  auto thr_g2s = g2s.get_thread_slice(tid);
  Tensor tAgA = thr_g2s.partition_S(gA);  // (CPY, CPY_M, CPY_W, load)
  Tensor tAsA = thr_g2s.partition_D(sA);  // (CPY, CPY_M, CPY_W, stage)
  Tensor cA = make_identity_tensor(
      Shape<Int<kNumThreads>, Int<kNumWordsPerLoad>>{});
  Tensor tAcA = thr_g2s.partition_S(cA);
  Tensor tApA = make_tensor<bool>(make_shape(size<1>(tAsA), size<2>(tAsA)));

  int const chunk_row_base = int(blockIdx.x) * kNumThreads;
  CUTLASS_PRAGMA_UNROLL
  for (int w = 0; w < size<2>(tAcA); ++w) {
    CUTLASS_PRAGMA_UNROLL
    for (int m = 0; m < size<1>(tAcA); ++m) {
      tApA(m, w) = (chunk_row_base + get<0>(tAcA(_0{}, m, w))) <
                   int(num_full_chunks);
    }
  }

  auto load_slice = [&](int load_idx, int stage) {
    if (load_idx < kNumLoads) {
      cute::copy_if(g2s, tApA, tAgA(_, _, _, load_idx), tAsA(_, _, _, stage));
    }
    cute::cp_async_fence();
  };

  CUTLASS_PRAGMA_UNROLL
  for (int s = 0; s < kNumStages - 1; ++s) {
    load_slice(s, s);
  }

  // This thread's chunk, and whether it is the trailing partial one.
  u32 const remainder = params.data_len % blake3::CHUNK_SIZE;
  u32 const last_chunk_size =
      (remainder == 0) ? blake3::CHUNK_SIZE : remainder;
  u32 const global_chunk_idx = blockIdx.x * kNumThreads + tid;
  bool const is_last_chunk = (global_chunk_idx == num_chunks - 1) &&
                             (last_chunk_size < blake3::CHUNK_SIZE);

  Tensor cv = make_tensor<uint32_t>(
      Layout<Shape<Int<blake3::CHAINING_VALUE_SIZE_U32>>>{});
  CUTLASS_PRAGMA_UNROLL
  for (int i = 0; i < blake3::CHAINING_VALUE_SIZE_U32; ++i) {
    cv(i) = c_key[i];
  }

  CUTLASS_PRAGMA_NO_UNROLL
  for (int load_idx = 0; load_idx < kNumLoads; ++load_idx) {
    cute::cp_async_wait<kNumStages - 2>();
    __syncthreads();

    int const stage = load_idx % kNumStages;
    load_slice(load_idx + kNumStages - 1,
               (load_idx + kNumStages - 1) % kNumStages);

    // Only this thread reads this row, so the overwrite needs no barrier.
    if (is_last_chunk) {
      load_partial_chunk_sm89<Kernel>(params, sA, tid, stage, load_idx,
                                      last_chunk_size);
    }

    CUTLASS_PRAGMA_UNROLL
    for (int block_in_load = 0; block_in_load < Kernel::kNumBlocksPerLoad;
         ++block_in_load) {
      int const block_idx = load_idx * Kernel::kNumBlocksPerLoad + block_in_load;
      if (is_single_chunk) {
        compress_block_sm89<Kernel, Kernel::kApplyRoot>(sA, cv, tid, stage,
                                                        block_in_load, block_idx);
      } else {
        compress_block_sm89<Kernel, false>(sA, cv, tid, stage, block_in_load,
                                           block_idx);
      }
    }
  }

  cute::cp_async_wait<0>();
  __syncthreads();

  CUTLASS_PRAGMA_UNROLL
  for (int i = 0; i < blake3::CHAINING_VALUE_SIZE_U32; ++i) {
    sLeaves(i, tid) = cv(i);
  }
  __syncthreads();

  // ---- Merkle reduction, unchanged from the Hopper kernel ---------------
  bool const is_last_block = (blockIdx.x == num_grid_blocks - 1);
  u32 const num_leaves = [&]() -> u32 {
    if (!is_last_block) return u32(kNumThreads);
    u32 const chunks_in_this_block = num_chunks % kNumThreads;
    return (chunks_in_this_block == 0) ? u32(kNumThreads) : chunks_in_this_block;
  }();

  if (!is_last_block) {
    merkle_tree_utils::compute_perfect_mt<false>(sLeaves, kNumThreads);
  } else if ((num_leaves & (num_leaves - 1)) == 0) {
    merkle_tree_utils::compute_perfect_mt<Kernel::kApplyRoot>(sLeaves,
                                                              num_leaves);
  } else {
    merkle_tree_utils::compute_blake_mt<Kernel::kApplyRoot>(sLeaves, num_leaves);
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

template <class Kernel>
cudaError_t run_merkle_tree_roots_sm89(typename Kernel::Arguments const& args,
                                       cudaStream_t stream = 0) {
  typename Kernel::Params const params = Kernel::to_underlying_arguments(args);
  auto kernel = ada_merkle_tree_roots<Kernel>;
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

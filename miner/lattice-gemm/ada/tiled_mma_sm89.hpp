#pragma once

// Ada (sm_89) MMA tiling for the NoisyGEMM mainloop.
//
// The production mainloop in ../csrc/gemm/ uses warpgroup MMA, which Ada does
// not have. The replacement is the SM80 int8 tensor-core atom. Which atom to
// pick, and how to tile it, is not a free choice: the accumulator's thread
// layout is consensus-visible.
//
// Each MMA thread runs an independent PoW attempt over the C elements that
// live in *its own* registers (see TileHashAccumulator in
// ../csrc/gemm/pow_utils.hpp). When a thread wins, the host signal header
// reports the tile-relative rows and columns it held, and those become the
// rows_pattern / cols_pattern of the submitted proof
// (lattice_gemm/helpers.py::extract_indices ->
//  zk-pow/src/v1/api/plain_proof.rs::list_to_pattern). Change the accumulator
// layout and you change which submatrix a nonce commits to.
//
// So the tiling below is chosen to reproduce the Hopper accumulator layout
// exactly, thread for thread. layout_equiv_sm89.cu proves that it does.

#include "cute/atom/mma_atom.hpp"
#include "cute/tensor.hpp"

namespace lattice::ada {

using namespace cute;

// The M x N tile the miner's rows_pattern / cols_pattern defaults describe.
// See miner/miner-base/src/miner_base/settings.py: tile_size_m, tile_size_n.
static constexpr int kConsensusTileM = 128;
static constexpr int kConsensusTileN = 256;

// int8 x int8 -> int32, K-major operands: the same arithmetic the warpgroup
// MMA performs, and the MMAType::Int7xInt7ToInt32 the verifier expects.
using MmaAtom = SM80_16x8x32_S32S8S8S32_TN;

// The K extent of one MMA instruction. Identical to the warpgroup atom's, which
// is what keeps the transcript reduction cadence (R / MMAAtom_K) unchanged.
static constexpr int kMmaAtomK = 32;

/// MMA tiling for an Ada CTA covering a TileM x TileN output tile.
///
/// Hopper splits TileM across warpgroups of 64 rows; each of the 4 warps in a
/// warpgroup then owns 16 rows. Ada has no warpgroups, so the same 16-rows-per-warp
/// split is expressed directly: TileM / 16 warps laid out along M.
template <int TileM_ = kConsensusTileM, int TileN_ = kConsensusTileN>
struct AdaMmaTiling {
  static constexpr int TileM = TileM_;
  static constexpr int TileN = TileN_;

  // One warp per 16 rows, matching the warp-level M split inside a Hopper
  // warpgroup. Anything else moves rows between threads.
  static_assert(TileM % 16 == 0, "TileM must be a multiple of the atom's 16 rows");
  static_assert(TileN % 8 == 0, "TileN must be a multiple of the atom's 8 columns");

  static constexpr int kNumWarps = TileM / 16;
  static constexpr int kNumMmaThreads = kNumWarps * 32;

  using AtomLayoutMNK = Layout<Shape<Int<kNumWarps>, _1, _1>>;
  using TiledMma = decltype(make_tiled_mma(MmaAtom{}, AtomLayoutMNK{}));

  // Registers of C per thread: 2 rows x (TileN / 4) columns.
  static constexpr int kAccumRegsPerThread = 2 * (TileN / 4);
};

using DefaultMmaTiling = AdaMmaTiling<kConsensusTileM, kConsensusTileN>;

}  // namespace lattice::ada

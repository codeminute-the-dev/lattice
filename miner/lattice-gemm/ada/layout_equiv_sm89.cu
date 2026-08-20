// Proves the Ada MMA tiling is consensus-equivalent to the Hopper one.
//
// The mainloop port replaces warpgroup MMA with the SM80 int8 atom. That swap
// is only safe if two things are preserved:
//
//   1. The accumulator's thread layout. Every MMA thread hashes the C elements
//      in its own registers and races for the PoW target on that transcript.
//      The winning thread's tile-relative rows and columns are submitted with
//      the proof, so a different layout means a different set of nonces --
//      and a proof whose rows_pattern / cols_pattern the verifier has to
//      accept as a periodic pattern.
//
//   2. The reduction cadence. The transcript is folded once every R/32 k-blocks
//      into a 16-word BLAKE3 message block. Ada cannot afford the production
//      bK=128, so bK has to shrink -- which changes how many k-blocks each tile
//      contributes, and therefore risks perturbing the schedule.
//
// This checks both, exhaustively, against the production configuration.
// It is a host program: cute layouts are compile-time objects, so no GPU is
// needed to run it.
//
//   nvcc -std=c++17 -x cu -arch=sm_89 -w --expt-relaxed-constexpr \
//     -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
//     ada/layout_equiv_sm89.cu -o /tmp/layout_equiv && /tmp/layout_equiv

#include <cstdio>
#include <set>
#include <vector>

#include "cute/tensor.hpp"
#include "cutlass/gemm/collective/collective_builder.hpp"

#include "tiled_mma_sm89.hpp"

using namespace cute;

// ---------------------------------------------------------------------------
// The production configuration, mirrored from the sources named below.
// ---------------------------------------------------------------------------

// miner/miner-base/src/miner_base/settings.py: tile_size_m/n/k, noise_rank.
static constexpr int kTileM = 128;
static constexpr int kTileN = 256;
static constexpr int kHopperTileK = 128;
static constexpr int kNoiseRank = 128;

// settings.py: rows_pattern -- the tile-relative rows one thread accumulates.
static const std::vector<int> kRowsPattern = {0, 8};

// settings.py: cols_pattern -- the tile-relative columns one thread accumulates.
static std::vector<int> cols_pattern() {
  std::vector<int> cols;
  for (int base = 0; base < kTileN; base += 8) {
    cols.push_back(base);
    cols.push_back(base + 1);
  }
  return cols;
}

// ../csrc/gemm/kernel_traits.hpp: TiledMma, for tile_size_m = 128.
static constexpr int kNumMmaWarpgroups = kTileM / 64;
using HopperTileShape_MNK = Shape<Int<kTileM>, Int<kTileN>, Int<kHopperTileK>>;
using HopperTiledMma = decltype(make_tiled_mma(
    GMMA::ss_op_selector<int8_t, int8_t, int32_t, HopperTileShape_MNK>(),
    Layout<Shape<Int<kNumMmaWarpgroups>, _1, _1>>{}));

using AdaTiling = lattice::ada::AdaMmaTiling<kTileM, kTileN>;
using AdaTiledMma = typename AdaTiling::TiledMma;

// ---------------------------------------------------------------------------

struct ThreadTile {
  std::vector<std::pair<int, int>> coords;  // register index -> (row, col)
  std::set<int> rows;
  std::set<int> cols;
};

template <class TiledMma>
static ThreadTile thread_tile(TiledMma mma, int thread_idx) {
  auto thr_mma = mma.get_thread_slice(thread_idx);
  Tensor cD = make_identity_tensor(Shape<Int<kTileM>, Int<kTileN>>{});
  Tensor tCcD = thr_mma.partition_C(cD);

  ThreadTile out;
  for (int i = 0; i < size(tCcD); ++i) {
    int row = get<0>(tCcD(i));
    int col = get<1>(tCcD(i));
    out.coords.emplace_back(row, col);
    out.rows.insert(row);
    out.cols.insert(col);
  }
  return out;
}

static std::vector<int> relative(std::set<int> const& values) {
  std::vector<int> out;
  int base = *values.begin();
  for (int v : values) out.push_back(v - base);
  return out;
}

static int failures = 0;

static void check(bool ok, char const* what) {
  if (!ok) {
    printf("  FAIL: %s\n", what);
    ++failures;
  }
}

// ---------------------------------------------------------------------------
// 1. Accumulator layout
// ---------------------------------------------------------------------------

static void check_accumulator_layout() {
  int const num_threads = size(HopperTiledMma{});
  printf("accumulator layout over a %dx%d tile, %d MMA threads\n", kTileM,
         kTileN, num_threads);

  check(num_threads == AdaTiling::kNumMmaThreads,
        "Ada thread count differs from Hopper's");

  std::vector<int> const expected_cols = cols_pattern();
  int mismatched_coords = 0;

  for (int t = 0; t < num_threads; ++t) {
    ThreadTile hopper = thread_tile(HopperTiledMma{}, t);
    ThreadTile ada = thread_tile(AdaTiledMma{}, t);

    if (hopper.coords.size() != ada.coords.size()) {
      printf("  FAIL: thread %d holds %zu elements on Hopper, %zu on Ada\n", t,
             hopper.coords.size(), ada.coords.size());
      ++failures;
      continue;
    }

    // Element-for-element, not just as a set: the transcript is an XOR fold,
    // so the set is what matters mathematically, but an identical register
    // order means the ported mainloop can reuse the reduction verbatim.
    for (size_t i = 0; i < hopper.coords.size(); ++i) {
      if (hopper.coords[i] != ada.coords[i]) ++mismatched_coords;
    }

    // extract_indices() submits sorted(set(rows)) x sorted(set(cols)) as the
    // proof's index lists, which is only the thread's actual element set if
    // the thread holds the full cross product.
    if (hopper.coords.size() != hopper.rows.size() * hopper.cols.size()) {
      printf("  FAIL: thread %d does not hold a full row x column cross product\n", t);
      ++failures;
    }

    if (relative(ada.rows) != kRowsPattern) {
      printf("  FAIL: thread %d rows do not match settings.py rows_pattern\n", t);
      ++failures;
    }
    if (relative(ada.cols) != expected_cols) {
      printf("  FAIL: thread %d cols do not match settings.py cols_pattern\n", t);
      ++failures;
    }
  }

  printf("  checked %d threads x %d registers, %d coordinate mismatches\n",
         num_threads, AdaTiling::kAccumRegsPerThread, mismatched_coords);
  check(mismatched_coords == 0, "Ada and Hopper disagree on some C coordinate");

  ThreadTile t0 = thread_tile(AdaTiledMma{}, 0);
  printf("  thread 0: %zu registers, rows {%d,%d}, cols {%d,%d,%d,%d,...}\n",
         t0.coords.size(), *t0.rows.begin(), *std::next(t0.rows.begin()),
         *t0.cols.begin(), *std::next(t0.cols.begin(), 1),
         *std::next(t0.cols.begin(), 2), *std::next(t0.cols.begin(), 3));
}

// ---------------------------------------------------------------------------
// 2. Transcript reduction cadence
// ---------------------------------------------------------------------------

// The bookkeeping of TileHashAccumulator in ../csrc/gemm/pow_utils.hpp,
// recorded as (global k-block, transcript word) rather than executed.
static std::vector<std::pair<int, int>> reduction_schedule(int tile_k, int R,
                                                           int K) {
  int const k_blocks_per_tile = tile_k / lattice::ada::kMmaAtomK;
  int const reduce_every_k = R / lattice::ada::kMmaAtomK;
  int const accums_per_tile = std::max(1, k_blocks_per_tile / reduce_every_k);
  int const last_full_k_block = K / lattice::ada::kMmaAtomK;
  int const msg_block_u32 = 16;  // blake3::MSG_BLOCK_SIZE_U32

  std::vector<std::pair<int, int>> events;
  int reduction_count = 0;
  int k_block_count = 0;

  for (int k_tile = 0; k_tile < K / tile_k; ++k_tile) {
    for (int k_block = 0; k_block < k_blocks_per_tile; ++k_block) {
      ++k_block_count;
      if ((k_block_count % reduce_every_k == 0) &&
          (k_block_count <= last_full_k_block)) {
        int const idx = k_block / reduce_every_k;
        events.emplace_back(k_block_count - 1, reduction_count + idx);
      }
    }
    if ((k_blocks_per_tile / reduce_every_k > 0) ||
        (k_block_count % reduce_every_k == 0)) {
      reduction_count = (reduction_count + accums_per_tile) % msg_block_u32;
    }
  }
  return events;
}

static void check_reduction_cadence() {
  int const K = 4096;
  printf("transcript cadence, K=%d rank=%d\n", K, kNoiseRank);

  auto const reference = reduction_schedule(kHopperTileK, kNoiseRank, K);
  printf("  bK=%3d (Hopper): %zu reductions\n", kHopperTileK, reference.size());
  check(!reference.empty(), "reference schedule is empty");

  for (int tile_k : {64, 32}) {
    auto const candidate = reduction_schedule(tile_k, kNoiseRank, K);
    bool const same = candidate == reference;
    printf("  bK=%3d (Ada)   : %zu reductions, schedule %s\n", tile_k,
           candidate.size(), same ? "identical" : "DIFFERS");
    check(same, "shrinking bK perturbs the transcript schedule");

    int const accums_per_tile = std::max(
        1, (tile_k / lattice::ada::kMmaAtomK) / (kNoiseRank / lattice::ada::kMmaAtomK));
    check(16 % accums_per_tile == 0,
          "accums_per_tile must divide the BLAKE3 message block");
  }
}

int main() {
  printf("Ada (sm_89) NoisyGEMM layout equivalence\n\n");
  check_accumulator_layout();
  printf("\n");
  check_reduction_cadence();
  printf("\n");

  if (failures == 0) {
    printf("PASS: the Ada tiling is consensus-equivalent to the Hopper one\n");
    return 0;
  }
  printf("FAIL: %d check(s) failed\n", failures);
  return 1;
}

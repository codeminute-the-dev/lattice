// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//! Work-function shape parameters — the retunable surface of Lattice's PoW.
//!
//! # Why this module exists
//!
//! Lattice does not try to prevent ASIC mining cryptographically. That is not
//! reliably achievable, and claiming otherwise would be dishonest. What Lattice
//! does instead is keep the *shape* of the matmul work function — tile
//! geometry, operand precision, and the noise rank that resists low-rank
//! shortcuts — collected in one clearly labelled, version-tagged place.
//!
//! The purpose is auditability of change. If network hashrate ever spikes in a
//! way that suggests specialised hardware has concentrated mining power, the
//! community response is a hardfork that retunes these values. Because they are
//! gathered here and re-exported from a single version tag, such a proposal is
//! a small diff a reviewer can read in one sitting, rather than an archaeology
//! expedition across the circuit.
//!
//! # What this module is NOT
//!
//! It is not a runtime configuration point. Nothing reads these values from a
//! config file, a flag, an environment variable, or chain state. They are
//! compile-time constants that must match the proving circuit exactly, and the
//! assertions at the bottom of this file fail the build if they ever drift
//! apart. Changing a value here changes consensus, and every node operator must
//! consciously adopt the resulting build.
//!
//! # Governance for retuning
//!
//! This is a social process, not a code path. See `docs/anti-asic.md`. In
//! outline: a maintainer publishes a hardfork proposal naming the new
//! parameters, the evidence of hardware concentration, and an activation
//! height; it is discussed in the open; nodes and miners adopt it by upgrading.
//! No key, no admin call, and no in-protocol vote can change these values.
//!
//! # Retuning checklist
//!
//! Any change here is a consensus break requiring a coordinated hardfork:
//!
//! 1. Bump [`WORK_FUNCTION_VERSION`].
//! 2. Update the constants below; the static assertions keep the circuit honest.
//! 3. Regenerate the proving/verifying key caches (`task build:zk-cache`).
//! 4. Set an activation height in `node/chaincfg/params.go`, following the
//!    existing fork-height pattern.
//! 5. Publish the proposal per `docs/anti-asic.md`.

use crate::circuit::lattice_program::{JACKPOT_SIZE, LROT_PER_TILE, TILE_D, TILE_H};

/// Version tag for the work-function parameter set.
///
/// Bump on any consensus-visible change to the values in this module. It exists
/// so a proposal, a release note, and a running binary can all name the same
/// parameter generation unambiguously.
pub const WORK_FUNCTION_VERSION: u32 = 1;

/// Tile depth (K dimension of one hardware multiply).
///
/// Together with [`TILE_HEIGHT`] this fixes the minimal multiply the circuit
/// commits to. Raising it deepens each accumulation and shifts the
/// compute-to-memory ratio; it is the primary knob if mining hardware turns out
/// to be memory-bound rather than arithmetic-bound.
pub const TILE_DEPTH: usize = TILE_D;

/// Tile height (M and N dimensions of one hardware multiply).
///
/// The minimal multiplier is `(TILE_HEIGHT × TILE_DEPTH) × (TILE_DEPTH ×
/// TILE_HEIGHT)`. Raising it widens the systolic shape a competitive miner must
/// build.
pub const TILE_HEIGHT: usize = TILE_H;

/// Operand precision, in bits, of the matmul inputs.
///
/// Matrix entries are signed 8-bit integers. This is the parameter most
/// directly tied to commodity ML accelerators: it is chosen so that ordinary
/// int8 tensor hardware is useful for mining, which is the point of
/// proof-of-useful-work. Changing it is the heaviest retune available and
/// affects the trace layout throughout the circuit.
pub const OPERAND_PRECISION_BITS: u32 = 8;

/// Minimum rank of the additive noise matrix.
///
/// The rank-penalty rule rejects proofs whose noise is lower-rank than this,
/// which is what stops a miner from cheaply factoring the noise away and
/// skipping the real work. Raising it raises the cost floor of any shortcut.
/// This value is asserted against the Rust enforcement path and the Go FFI
/// binding, both of which must agree.
pub const MIN_NOISE_RANK: usize = crate::api::sanity_checks::PENALTY_BASE_RANK;

/// Number of u32 words in the jackpot buffer that gates block acceptance.
pub const JACKPOT_WORDS: usize = JACKPOT_SIZE;

/// Left-rotation applied per tile when folding tile results into the bit
/// register. Part of the work function's diffusion, retuned only alongside the
/// tile geometry.
pub const ROTATION_PER_TILE: u32 = LROT_PER_TILE;

/// Human-readable summary, for logs, `getinfo`-style RPC output, and hardfork
/// proposals.
pub fn describe() -> String {
    format!(
        "work-function v{WORK_FUNCTION_VERSION}: tile {TILE_HEIGHT}x{TILE_DEPTH}, \
         int{OPERAND_PRECISION_BITS} operands, min noise rank {MIN_NOISE_RANK}, \
         jackpot {JACKPOT_WORDS} words, rot/tile {ROTATION_PER_TILE}"
    )
}

// These re-exports are the whole point of the module: if the circuit's own
// constants are ever edited without updating this file (or vice versa), the
// build fails here rather than silently shipping a chain split.
const _: () = assert!(TILE_DEPTH == TILE_D);
const _: () = assert!(TILE_HEIGHT == TILE_H);
const _: () = assert!(JACKPOT_WORDS == JACKPOT_SIZE);
const _: () = assert!(ROTATION_PER_TILE == LROT_PER_TILE);
const _: () = assert!(OPERAND_PRECISION_BITS == 8);
const _: () = assert!(MIN_NOISE_RANK == crate::api::sanity_checks::PENALTY_BASE_RANK);

// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

// Emission — Lattice's sawtooth block reward schedule.
//
// Lattice inherits Pearl's hyperbolic subsidy curve and wraps it in a
// repeating epoch. Inside one epoch, the reward for the i-th block of that
// epoch (i counted from 1) is
//
//	reward(i) = S*k / ((i+k) * (i-1+k))          [in cells]
//
// which starts at exactly S/(1+k) and decays smoothly toward zero. Summed
// over an epoch it telescopes to the closed form
//
//	cumulative(i) = S*i / (i+k)
//
// so the curve converges toward the reference supply S without ever reaching
// it. Rather than let the reward decay forever, Lattice resets: once an epoch
// has minted ResetThreshold cells, the schedule jumps back to the starting
// reward and the curve restarts from i = 1. This repeats indefinitely, so
// there is no maximum supply and no cliff where rewards stop.
//
// The reset is expressed two equivalent ways, and TestEpochBlocksMatchesThreshold
// asserts they agree exactly:
//
//   - Economically: reset when cumulative emission since the last reset
//     crosses ResetThreshold. This is the rule the design is stated in.
//   - Mechanically: reset every EpochBlocks blocks. This is what consensus
//     actually evaluates, because it is O(1) and cannot drift.
//
// EpochBlocks is therefore not an independent knob — it is the smallest i for
// which the truncated integer sum of reward(1..i) reaches ResetThreshold. All
// four values are hardcoded consensus constants. Nothing in the protocol can
// change them at runtime: there is no governance hook, no creator key, and no
// post-launch adjustment path. Changing them requires a hardfork that every
// node operator must consciously adopt.
//
// Resets only ever affect the *future* reward schedule. They do not touch
// existing balances, already-minted supply, or any historical block.
type EmissionParams struct {
	// Constant is k, the emission constant, in blocks. Larger k means a
	// flatter, more gradual decay.
	Constant int64

	// ReferenceSupply is S, in cells. It is the value the emission curve
	// converges toward within a single epoch. It is a shape parameter, not
	// a supply cap: because epochs repeat, total supply grows without bound.
	ReferenceSupply int64

	// ResetThreshold is T, in cells: the cumulative emission within one
	// epoch that triggers a reset of the reward schedule.
	ResetThreshold int64

	// EpochBlocks is L, the number of blocks in one emission epoch. Derived
	// from the three values above and verified by test; see the type comment.
	EpochBlocks int32
}

// InitialSubsidy returns the reward, in cells, of the first block of any
// epoch — the peak the schedule resets back to.
func (e *EmissionParams) InitialSubsidy() int64 {
	if e.Constant <= 0 {
		return 0
	}
	return e.ReferenceSupply / (1 + e.Constant)
}

// mainNetEmission is the Lattice mainnet emission schedule.
//
// Tuned so that, at the 40 second target block time:
//
//	reward starts at exactly 2 LATT
//	reward after 1 year   ~ 1.9223 LATT
//	reward at epoch end   ~ 1.7147 LATT   (a gradual ~14% taper, not a halving)
//	one epoch             = 3,153,600 blocks ~ 4.00 years
//	one epoch mints       = 5,840,000 LATT
//
// k is 39,420,000 blocks, which is 50 years of blocks at 40s — deliberately
// far longer than an epoch, which is what keeps the in-epoch decay gentle.
var mainNetEmission = EmissionParams{
	Constant:        39_420_000,
	ReferenceSupply: 78_840_002 * 1e8,
	ResetThreshold:  5_840_000 * 1e8,
	EpochBlocks:     3_153_600,
}

// testNetEmission compresses the same curve shape into a short epoch so that
// resets can actually be observed and dogfooded on a test network. The ratio
// EpochBlocks/Constant is held at 0.08, identical to mainnet, so the reward
// decays through exactly the same shape — just ~315x faster.
var testNetEmission = EmissionParams{
	Constant:        125_000,
	ReferenceSupply: 250_002 * 1e8,
	ResetThreshold:  1_851_866_661_655,
	EpochBlocks:     10_000,
}

// simNetEmission compresses the curve further still, so a reset is reachable
// within seconds of solo mining on simnet/regtest.
var simNetEmission = EmissionParams{
	Constant:        2_500,
	ReferenceSupply: 5_002 * 1e8,
	ResetThreshold:  37_051_851_754,
	EpochBlocks:     200,
}

// MainNetEmission returns the mainnet emission schedule. It is the fallback
// used by consensus helpers when no chain parameters are supplied.
func MainNetEmission() *EmissionParams {
	return &mainNetEmission
}

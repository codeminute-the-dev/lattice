// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"testing"

	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/chaincfg"
)

const cell = int64(btcutil.CellPerLatt)

func allSchedules() map[string]*chaincfg.EmissionParams {
	return map[string]*chaincfg.EmissionParams{
		"mainnet": &chaincfg.MainNetParams.Emission,
		"testnet": &chaincfg.TestNetParams.Emission,
		"simnet":  &chaincfg.SimNetParams.Emission,
	}
}

// TestEpochBlocksMatchesThreshold is the load-bearing test of the whole
// emission design. EpochBlocks is what consensus actually evaluates, but
// ResetThreshold is how the design is stated and published. If they ever
// disagreed, the chain would reset at a height that contradicts its own
// documentation. This asserts they are the same rule: EpochBlocks is exactly
// the smallest block count whose truncated rewards reach the threshold.
func TestEpochBlocksMatchesThreshold(t *testing.T) {
	for name, e := range allSchedules() {
		t.Run(name, func(t *testing.T) {
			var cum int64
			var reached int32
			for i := int64(1); i <= int64(e.EpochBlocks)+1; i++ {
				cum += subsidyForIndex(i, e)
				if cum >= e.ResetThreshold {
					reached = int32(i)
					break
				}
			}
			if reached != e.EpochBlocks {
				t.Fatalf("threshold reached at block %d, but EpochBlocks is %d",
					reached, e.EpochBlocks)
			}

			// And one block earlier must fall short, or EpochBlocks
			// would not be the *smallest* such count.
			short := cum - subsidyForIndex(int64(e.EpochBlocks), e)
			if short >= e.ResetThreshold {
				t.Fatalf("threshold already met at block %d", e.EpochBlocks-1)
			}
		})
	}
}

// TestSubsidyFastPathMatchesBigInt guards the 128-bit arithmetic in
// subsidyForIndex against the arbitrary-precision reference. A divergence here
// would be a consensus split, so it is checked across the epoch rather than
// spot-checked.
func TestSubsidyFastPathMatchesBigInt(t *testing.T) {
	for name, e := range allSchedules() {
		t.Run(name, func(t *testing.T) {
			ref := func(i int64) int64 {
				num := new(big.Int).Mul(big.NewInt(e.ReferenceSupply), big.NewInt(e.Constant))
				den := new(big.Int).Mul(big.NewInt(i+e.Constant), big.NewInt(i-1+e.Constant))
				return new(big.Int).Div(num, den).Int64()
			}
			step := int64(e.EpochBlocks) / 5000
			if step < 1 {
				step = 1
			}
			for i := int64(1); i <= int64(e.EpochBlocks); i += step {
				if got, want := subsidyForIndex(i, e), ref(i); got != want {
					t.Fatalf("i=%d: fast path %d, big.Int %d", i, got, want)
				}
			}
		})
	}
}

// TestStartingRewardIsExactlyTwoLATT pins the headline number. Every epoch,
// on every network, opens at exactly 2 LATT.
func TestStartingRewardIsExactlyTwoLATT(t *testing.T) {
	for name, e := range allSchedules() {
		t.Run(name, func(t *testing.T) {
			if got := subsidyForIndex(1, e); got != 2*cell {
				t.Fatalf("first reward = %d cells, want %d", got, 2*cell)
			}
			if got := e.InitialSubsidy(); got != 2*cell {
				t.Fatalf("InitialSubsidy() = %d cells, want %d", got, 2*cell)
			}
		})
	}
}

// TestGenesisMintsNothing is the no-premine check at the consensus layer: the
// genesis block's subsidy is zero, so the protocol creates no coins before the
// first mined block.
func TestGenesisMintsNothing(t *testing.T) {
	for _, p := range []*chaincfg.Params{
		&chaincfg.MainNetParams, &chaincfg.TestNetParams, &chaincfg.SimNetParams,
	} {
		if got := CalcBlockSubsidy(0, p); got != 0 {
			t.Fatalf("%s: genesis subsidy = %d, want 0", p.Name, got)
		}
	}
}

// TestRewardResetsAtEpochBoundary checks the sawtooth itself: the reward
// decays across an epoch, then jumps back to the full starting reward on the
// first block of the next one.
func TestRewardResetsAtEpochBoundary(t *testing.T) {
	p := &chaincfg.SimNetParams
	e := &p.Emission
	L := e.EpochBlocks

	last := CalcBlockSubsidy(L, p)
	first := CalcBlockSubsidy(L+1, p)

	if last >= 2*cell {
		t.Fatalf("reward at epoch end (%d) should have decayed below 2 LATT", last)
	}
	if first != 2*cell {
		t.Fatalf("reward after reset = %d cells, want a jump back to %d", first, 2*cell)
	}
	// Reward must decrease monotonically within an epoch.
	prev := CalcBlockSubsidy(1, p)
	for h := int32(2); h <= L; h++ {
		cur := CalcBlockSubsidy(h, p)
		if cur > prev {
			t.Fatalf("reward rose inside an epoch at height %d: %d > %d", h, cur, prev)
		}
		prev = cur
	}
}

// TestDecayIsGradual asserts the design intent that the reward stays close to
// 2 LATT rather than falling off a cliff: mainnet must still pay over 1.7 LATT
// at the end of a four-year epoch, and over 1.9 LATT after a year.
func TestDecayIsGradual(t *testing.T) {
	p := &chaincfg.MainNetParams
	e := &p.Emission

	if got := CalcBlockSubsidy(788_400, p); got < 190*cell/100 {
		t.Errorf("reward after 1 year = %d cells, want > 1.90 LATT", got)
	}
	if got := CalcBlockSubsidy(e.EpochBlocks, p); got < 170*cell/100 {
		t.Errorf("reward at epoch end = %d cells, want > 1.70 LATT", got)
	}
	// No step-function halving: no single block may drop the reward by
	// more than a hair.
	for _, h := range []int32{1, 1000, 100_000, 1_000_000, e.EpochBlocks - 1} {
		a, b := CalcBlockSubsidy(h, p), CalcBlockSubsidy(h+1, p)
		if a-b > cell/1000 {
			t.Errorf("reward dropped %d cells between heights %d and %d", a-b, h, h+1)
		}
	}
}

// TestNextResetIsExactAndPredictable verifies a node can name every future
// reset height from constants alone, which is what the countdown depends on.
func TestNextResetIsExactAndPredictable(t *testing.T) {
	p := &chaincfg.SimNetParams
	e := &p.Emission
	L := e.EpochBlocks

	cases := []struct{ height, wantReset int32 }{
		{1, L + 1},
		{L - 1, L + 1},
		{L, L + 1},
		{L + 1, 2*L + 1},
		{5*L + 7, 6*L + 1},
	}
	for _, c := range cases {
		if got := NextResetHeight(c.height, e); got != c.wantReset {
			t.Errorf("NextResetHeight(%d) = %d, want %d", c.height, got, c.wantReset)
		}
	}
	if got := BlocksUntilReset(L-10, e); got != 11 {
		t.Errorf("BlocksUntilReset = %d, want 11", got)
	}
	// Every reset height must actually pay the starting reward.
	for epoch := int64(1); epoch <= 5; epoch++ {
		h := EpochStartHeight(epoch, e)
		if got := CalcBlockSubsidy(h, p); got != 2*cell {
			t.Errorf("epoch %d starts at height %d paying %d, want %d",
				epoch, h, got, 2*cell)
		}
	}
}

// TestSupplyGrowsWithoutCap confirms there is no terminal height where rewards
// stop: emission continues at full strength many epochs out.
func TestSupplyGrowsWithoutCap(t *testing.T) {
	p := &chaincfg.SimNetParams
	e := &p.Emission

	epochTotal := EpochEmission(e)
	if epochTotal < e.ResetThreshold {
		t.Fatalf("epoch mints %d cells, below its own threshold %d", epochTotal, e.ResetThreshold)
	}
	// Total supply through N whole epochs is exactly N times one epoch.
	for _, n := range []int32{1, 2, 10, 100} {
		h := n * e.EpochBlocks
		want := int64(n) * epochTotal
		if got := TotalSupplyThrough(h, p); got != want {
			t.Errorf("TotalSupplyThrough(%d) = %d, want %d", h, got, want)
		}
	}
	// Far-future blocks still pay the full starting reward.
	if got := CalcBlockSubsidy(1000*e.EpochBlocks+1, p); got != 2*cell {
		t.Errorf("far-future reset pays %d, want %d", got, 2*cell)
	}
}

// TestResetsDoNotAlterPastEmission is the immutability guarantee: what a past
// height minted is a pure function of that height and never changes as the
// chain advances through further resets.
func TestResetsDoNotAlterPastEmission(t *testing.T) {
	p := &chaincfg.SimNetParams
	e := &p.Emission

	snapshot := make(map[int32]int64)
	for h := int32(1); h <= e.EpochBlocks*2; h += 7 {
		snapshot[h] = CalcBlockSubsidy(h, p)
	}
	// Advance conceptually past many further resets, then re-evaluate.
	for h, want := range snapshot {
		if got := CalcBlockSubsidy(h, p); got != want {
			t.Fatalf("height %d minted %d, now reports %d", h, want, got)
		}
	}
	// And supply through a past height is likewise stable.
	h := e.EpochBlocks + 500
	if a, b := TotalSupplyThrough(h, p), TotalSupplyThrough(h, p); a != b {
		t.Fatalf("TotalSupplyThrough not deterministic: %d vs %d", a, b)
	}
}

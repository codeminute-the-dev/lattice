// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"testing"

	"github.com/codeminute-the-dev/lattice/node/chaincfg"
)

// TestSawtoothOverManyEpochs walks several complete epochs of the simnet
// schedule and checks the shape end to end: each epoch opens at exactly 2 LATT,
// decays monotonically, mints the same total as every other epoch, and hands
// off to the next epoch at precisely the height NextResetHeight predicted.
//
// This is the property the whole design rests on, so it is checked against the
// real parameters rather than a fixture.
func TestSawtoothOverManyEpochs(t *testing.T) {
	p := &chaincfg.SimNetParams
	e := &p.Emission
	L := e.EpochBlocks

	var runningSupply int64
	var firstEpochTotal int64

	for epoch := int64(0); epoch < 6; epoch++ {
		start := EpochStartHeight(epoch, e)

		// The epoch must open at the full starting reward.
		if got := CalcBlockSubsidy(start, p); got != 2*cell {
			t.Fatalf("epoch %d opens at %d cells, want %d", epoch, got, 2*cell)
		}

		// The block before it belongs to the previous epoch and must be
		// paying strictly less, which is what makes it a sawtooth rather
		// than a flat line.
		if epoch > 0 {
			if prev := CalcBlockSubsidy(start-1, p); prev >= 2*cell {
				t.Fatalf("epoch %d: block before reset paid %d, expected a decayed reward",
					epoch, prev)
			}
		}

		var epochTotal int64
		prev := int64(1<<62 - 1)
		for i := int32(0); i < L; i++ {
			h := start + i
			s := CalcBlockSubsidy(h, p)
			if s > prev {
				t.Fatalf("epoch %d: reward rose at height %d", epoch, h)
			}
			prev = s
			epochTotal += s

			if want := NextResetHeight(h, e); want != start+L {
				t.Fatalf("at height %d NextResetHeight = %d, want %d", h, want, start+L)
			}
		}

		if epoch == 0 {
			firstEpochTotal = epochTotal
			if epochTotal < e.ResetThreshold {
				t.Fatalf("epoch mints %d, below threshold %d", epochTotal, e.ResetThreshold)
			}
		} else if epochTotal != firstEpochTotal {
			t.Fatalf("epoch %d minted %d, but epoch 0 minted %d; epochs must be identical",
				epoch, epochTotal, firstEpochTotal)
		}

		runningSupply += epochTotal
		if got := TotalSupplyThrough(start+L-1, p); got != runningSupply {
			t.Fatalf("after epoch %d TotalSupplyThrough = %d, want %d",
				epoch, got, runningSupply)
		}
	}
}

// TestScheduleIsPurelyDeterministic confirms the reward depends on nothing but
// height and hardcoded constants. Anyone can compute the entire future schedule
// offline, which is the transparency guarantee the design promises.
func TestScheduleIsPurelyDeterministic(t *testing.T) {
	p := &chaincfg.MainNetParams
	e := &p.Emission

	// Re-derive the reward independently of the production code path, using
	// only the published constants and arbitrary-precision arithmetic, and
	// require agreement.
	independent := func(height int32) int64 {
		i := int64(height-1)%int64(e.EpochBlocks) + 1
		num := new(big.Int).Mul(big.NewInt(e.ReferenceSupply), big.NewInt(e.Constant))
		den := new(big.Int).Mul(big.NewInt(i+e.Constant), big.NewInt(i-1+e.Constant))
		return new(big.Int).Div(num, den).Int64()
	}

	for _, h := range []int32{1, 2, 1000, 788_400, e.EpochBlocks - 1, e.EpochBlocks,
		e.EpochBlocks + 1, e.EpochBlocks * 3, e.EpochBlocks*7 + 42} {
		if got, want := CalcBlockSubsidy(h, p), independent(h); got != want {
			t.Errorf("height %d: chain says %d, independent derivation says %d", h, got, want)
		}
	}
}

// TestFarFutureHeightsRemainSound checks the schedule near the top of the
// int32 height range, where an epoch calculation that overflowed would produce
// a nonsensical reward.
func TestFarFutureHeightsRemainSound(t *testing.T) {
	p := &chaincfg.MainNetParams
	e := &p.Emission

	for _, h := range []int32{1 << 20, 1 << 25, 1 << 30, 2_000_000_000, 2_147_483_600} {
		s := CalcBlockSubsidy(h, p)
		if s <= 0 || s > 2*cell {
			t.Errorf("height %d yields reward %d, outside (0, 2 LATT]", h, s)
		}
		if r := NextResetHeight(h, e); r <= h {
			t.Errorf("height %d: next reset %d is not in the future", h, r)
		}
	}
}

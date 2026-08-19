// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math"
	"math/big"
	"math/bits"
	"time"

	"github.com/codeminute-the-dev/lattice/node/chaincfg"
)

// See chaincfg/emission.go for the full description of the sawtooth reward
// schedule. This file implements it.
//
// Every function here is a pure function of block height and the hardcoded
// chain parameters. Nothing consults chain state, peer input, wallet state, or
// any operator-supplied value, so any two nodes agree on the entire past and
// future reward schedule without communicating.

// EpochIndex returns the zero-based emission epoch containing height. Epoch 0
// begins at height 1 (the genesis block, height 0, mints nothing).
func EpochIndex(height int32, e *chaincfg.EmissionParams) int64 {
	if height < 1 || e.EpochBlocks <= 0 {
		return 0
	}
	return int64(height-1) / int64(e.EpochBlocks)
}

// HeightInEpoch returns the 1-based position of height within its epoch. It is
// the i fed to the reward curve, so it resets to 1 at every reset boundary.
func HeightInEpoch(height int32, e *chaincfg.EmissionParams) int64 {
	if height < 1 || e.EpochBlocks <= 0 {
		return 0
	}
	return int64(height-1)%int64(e.EpochBlocks) + 1
}

// EpochStartHeight returns the height of the first block of the given epoch —
// that is, the height at which the reward jumped back to the starting subsidy.
//
// Block heights are int32, so an epoch far enough out would overflow. That
// takes on the order of 2.1 billion blocks (millennia at a 40 second target),
// but the result saturates at MaxInt32 rather than wrapping to a negative
// height, so callers near the ceiling still get a sane, ordered answer.
func EpochStartHeight(epoch int64, e *chaincfg.EmissionParams) int32 {
	start := epoch*int64(e.EpochBlocks) + 1
	if start > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(start)
}

// NextResetHeight returns the height of the next block whose reward resets to
// the starting subsidy, given the chain is currently at height.
//
// This is exact, not an estimate: it is fixed arithmetic on hardcoded
// constants, so a node can state the height of every future reset — the
// millionth one included — the moment it starts up.
func NextResetHeight(height int32, e *chaincfg.EmissionParams) int32 {
	if e.EpochBlocks <= 0 {
		return 0
	}
	if height < 1 {
		return EpochStartHeight(1, e)
	}
	return EpochStartHeight(EpochIndex(height, e)+1, e)
}

// BlocksUntilReset returns how many blocks remain until the next reset.
func BlocksUntilReset(height int32, e *chaincfg.EmissionParams) int32 {
	return NextResetHeight(height, e) - height
}

// subsidyForIndex evaluates the hyperbolic curve at in-epoch index i:
//
//	reward(i) = S*k / ((i+k) * (i-1+k))
//
// The numerator S*k overflows int64 on mainnet parameters, so the product is
// carried in 128 bits. The denominator comfortably fits in 64 bits for any
// realistic epoch length, but both the 128-bit precondition and the fit are
// checked, and a big.Int path takes over if either fails. The two paths are
// asserted to agree in TestSubsidyFastPathMatchesBigInt.
func subsidyForIndex(i int64, e *chaincfg.EmissionParams) int64 {
	if i < 1 || e.Constant <= 0 || e.ReferenceSupply <= 0 {
		return 0
	}

	a := i + e.Constant
	b := i - 1 + e.Constant

	// Denominator must fit in 64 bits for the fast path.
	if a > 0 && b > 0 && a <= (1<<31)-1 && b <= (1<<31)-1 {
		hi, lo := bits.Mul64(uint64(e.ReferenceSupply), uint64(e.Constant))
		den := uint64(a) * uint64(b)
		// bits.Div64 panics unless hi < den; on these parameters it is,
		// but never rely on that without checking.
		if hi < den {
			q, _ := bits.Div64(hi, lo, den)
			return int64(q)
		}
	}

	num := new(big.Int).Mul(
		big.NewInt(e.ReferenceSupply),
		big.NewInt(e.Constant),
	)
	den := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	return new(big.Int).Div(num, den).Int64()
}

// CalcBlockSubsidy returns the reward, in cells, for a block at the given
// height. This is consensus-critical: validate.go checks every coinbase
// against it, so a node that computed it differently would fork itself off
// the network.
func CalcBlockSubsidy(height int32, chainParams *chaincfg.Params) int64 {
	// The genesis block mints nothing. Lattice has no premine and no
	// founder allocation: there is no height at which the protocol creates
	// coins outside this function, and this function returns 0 here.
	if height == 0 {
		return 0
	}

	e := emissionParams(chainParams)
	return subsidyForIndex(HeightInEpoch(height, e), e)
}

// EpochEmission returns the exact total number of cells minted by one complete
// epoch, summing the truncated per-block rewards rather than using the
// continuous closed form.
func EpochEmission(e *chaincfg.EmissionParams) int64 {
	var total int64
	for i := int64(1); i <= int64(e.EpochBlocks); i++ {
		total += subsidyForIndex(i, e)
	}
	return total
}

// SupplyMintedInEpochThrough returns the exact cells minted from the start of
// height's epoch up to and including height.
func SupplyMintedInEpochThrough(height int32, e *chaincfg.EmissionParams) int64 {
	var total int64
	for i := int64(1); i <= HeightInEpoch(height, e); i++ {
		total += subsidyForIndex(i, e)
	}
	return total
}

// TotalSupplyThrough returns the exact cells minted by the whole chain up to
// and including height. Because every epoch mints an identical amount, the
// completed epochs collapse to a multiplication and only the current partial
// epoch is summed.
//
// This counts issuance only. It is not a ledger balance and nothing here can
// alter one: coins already mined are unaffected by any future reset.
func TotalSupplyThrough(height int32, chainParams *chaincfg.Params) int64 {
	if height < 1 {
		return 0
	}
	e := emissionParams(chainParams)
	return EpochIndex(height, e)*EpochEmission(e) +
		SupplyMintedInEpochThrough(height, e)
}

// EstimatedTimeUntilReset returns the expected wall-clock time to the next
// reset, at the network's target block time.
//
// Unlike the reset height, this is genuinely an estimate: it assumes blocks
// land at exactly the target interval, and real block times vary with hashrate
// and difficulty retargeting.
func EstimatedTimeUntilReset(height int32, chainParams *chaincfg.Params) time.Duration {
	e := emissionParams(chainParams)
	return time.Duration(BlocksUntilReset(height, e)) * chainParams.TargetTimePerBlock
}

// emissionParams returns the emission schedule for chainParams, falling back
// to mainnet's when a caller supplies nil (some tests construct chains without
// a full parameter set).
func emissionParams(chainParams *chaincfg.Params) *chaincfg.EmissionParams {
	if chainParams == nil || chainParams.Emission.EpochBlocks <= 0 {
		return chaincfg.MainNetEmission()
	}
	return &chainParams.Emission
}

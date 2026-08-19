// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/codeminute-the-dev/lattice/node/wire"
	"github.com/stretchr/testify/require"
)

// TestInvalidHashStr ensures the newShaHashFromStr function panics when used to
// with an invalid hash string.
func TestInvalidHashStr(t *testing.T) {
	require.Panics(t, func() {
		newHashFromStr("banana")
	}, "Expected panic for invalid hash")
}

// TestMustRegisterPanic ensures the mustRegister function panics when used to
// register an invalid network.
func TestMustRegisterPanic(t *testing.T) {
	t.Parallel()

	// Intentionally try to register duplicate params to force a panic.
	require.Panics(t, func() {
		mustRegister(&MainNetParams)
	}, "mustRegister did not panic as expected")
}

func TestRegisterHDKeyID(t *testing.T) {
	t.Parallel()

	// Ref: https://github.com/satoshilabs/slips/blob/master/slip-0132.md
	hdKeyIDZprv := []byte{0x02, 0xaa, 0x7a, 0x99}
	hdKeyIDZpub := []byte{0x02, 0xaa, 0x7e, 0xd3}

	err := RegisterHDKeyID(hdKeyIDZpub, hdKeyIDZprv)
	require.NoError(t, err, "RegisterHDKeyID")

	got, err := HDPrivateKeyToPublicKeyID(hdKeyIDZprv)
	require.NoError(t, err, "HDPrivateKeyToPublicKeyID")
	require.Equal(t, hdKeyIDZpub, got, "HDPrivateKeyToPublicKeyID result mismatch")
}

func TestInvalidHDKeyID(t *testing.T) {
	t.Parallel()

	prvValid := []byte{0x02, 0xaa, 0x7a, 0x99}
	pubValid := []byte{0x02, 0xaa, 0x7e, 0xd3}
	prvInvalid := []byte{0x00}
	pubInvalid := []byte{0x00}

	err := RegisterHDKeyID(pubInvalid, prvValid)
	require.ErrorIs(t, err, ErrInvalidHDKeyID)

	err = RegisterHDKeyID(pubValid, prvInvalid)
	require.ErrorIs(t, err, ErrInvalidHDKeyID)

	err = RegisterHDKeyID(pubInvalid, prvInvalid)
	require.ErrorIs(t, err, ErrInvalidHDKeyID)

	// FIXME: The error type should be changed to ErrInvalidHDKeyID.
	_, err = HDPrivateKeyToPublicKeyID(prvInvalid)
	require.ErrorIs(t, err, ErrUnknownHDKeyID)
}

func TestSigNetPowLimit(t *testing.T) {
	// sigNetPowLimit should be 2^228 - 1 (7 leading hex zeros followed by 57 f's)
	expectedPowLimitHex, err := hex.DecodeString(
		"0000000fffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	)
	require.NoError(t, err)
	expectedPowLimit := new(big.Int).SetBytes(expectedPowLimitHex)
	require.Equal(t, 0, sigNetPowLimit.Cmp(expectedPowLimit),
		"Signet PoW limit (%s) not equal to expected 2^228-1 (%s)",
		sigNetPowLimit.Text(16), expectedPowLimit.Text(16))

	// The genesis block Bits (0x1d0fffff) is the compact representation.
	// Compact format has limited precision (24-bit mantissa), so it yields
	// 0x0fffff000... rather than 0x0ffff...fff. Verify the expected compact value.
	expectedBitsTargetHex, err := hex.DecodeString(
		"0000000fffff0000000000000000000000000000000000000000000000000000",
	)
	require.NoError(t, err)
	expectedBitsTarget := new(big.Int).SetBytes(expectedBitsTargetHex)
	actualBitsTarget := compactToBig(sigNetGenesisBlock.BlockHeader().Bits)
	require.Equal(t, 0, actualBitsTarget.Cmp(expectedBitsTarget),
		"Signet genesis Bits target (%s) not equal to expected (%s)",
		actualBitsTarget.Text(16), expectedBitsTarget.Text(16))
}

// TestSigNetMagic makes sure that the default signet has the expected Lattice
// network magic.
func TestSigNetMagic(t *testing.T) {
	require.Equal(t, wire.SigNet, SigNetParams.Net)
}

// TestMoEForkActivation verifies the strict cutover at the MoE hardfork
// activation height: V1 before the fork, V2 at and after it.
func TestMoEForkActivation(t *testing.T) {
	const forkHeight = int32(100)
	p := Params{MoEForkHeight: forkHeight}

	tests := []struct {
		name        string
		height      int32
		wantActive  bool
		wantVersion wire.CertificateVersion
	}{
		{"genesis", 0, false, wire.CertificateVersionV1},
		{"just before fork", forkHeight - 1, false, wire.CertificateVersionV1},
		{"at fork height", forkHeight, true, wire.CertificateVersionV2},
		{"after fork height", forkHeight + 1, true, wire.CertificateVersionV2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantActive, p.IsMoEForkActive(tt.height))
			require.Equal(t, tt.wantVersion, p.RequiredCertVersion(tt.height))
		})
	}
}

// TestMoEForkDisabled verifies that a zero MoEForkHeight disables the fork at
// every height (the V1 certificate is always required).
func TestMoEForkDisabled(t *testing.T) {
	p := Params{MoEForkHeight: 0}
	for _, height := range []int32{0, 1, 100, 1_000_000} {
		require.False(t, p.IsMoEForkActive(height))
		require.Equal(t, wire.CertificateVersionV1, p.RequiredCertVersion(height))
	}
}

// TestSaltedSeedForkActivation verifies the strict cutover at the salted
// noise-seed hardfork activation height: V2 before the fork (with the MoE fork
// active), V3 at and after it.
func TestSaltedSeedForkActivation(t *testing.T) {
	const forkHeight = int32(200)
	p := Params{MoEForkHeight: 100, SaltedSeedForkHeight: forkHeight}

	tests := []struct {
		name        string
		height      int32
		wantActive  bool
		wantVersion wire.CertificateVersion
	}{
		{"just before fork", forkHeight - 1, false, wire.CertificateVersionV2},
		{"at fork height", forkHeight, true, wire.CertificateVersionV3},
		{"after fork height", forkHeight + 1, true, wire.CertificateVersionV3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantActive, p.IsSaltedSeedForkActive(tt.height))
			require.Equal(t, tt.wantVersion, p.RequiredCertVersion(tt.height))
		})
	}
}

// TestSaltedSeedForkDisabled verifies that a zero SaltedSeedForkHeight disables
// the fork at every height (the version follows the MoE fork schedule).
func TestSaltedSeedForkDisabled(t *testing.T) {
	p := Params{MoEForkHeight: 1, SaltedSeedForkHeight: 0}
	for _, height := range []int32{1, 100, 1_000_000} {
		require.False(t, p.IsSaltedSeedForkActive(height))
		require.Equal(t, wire.CertificateVersionV2, p.RequiredCertVersion(height))
	}
}

// TestRankPenaltyForkActivation verifies the activation boundary of the
// rank-penalty softfork, including the disabled case.
func TestRankPenaltyForkActivation(t *testing.T) {
	const forkHeight = int32(100)
	enabled := Params{RankPenaltyForkHeight: forkHeight}

	tests := []struct {
		name       string
		height     int32
		wantActive bool
	}{
		{"genesis", 0, false},
		{"just before fork", forkHeight - 1, false},
		{"at fork height", forkHeight, true},
		{"after fork height", forkHeight + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantActive, enabled.IsRankPenaltyForkActive(tt.height))
		})
	}

	disabled := Params{RankPenaltyForkHeight: 0}
	for _, height := range []int32{0, 1, 100, 1_000_000} {
		require.False(t, disabled.IsRankPenaltyForkActive(height))
	}
}

// TestShippedNetworksRankPenaltyOrdering asserts the invariant the rule relies
// on: only V2 certificates carry a noise rank, so the rank-penalty softfork must
// never activate before the V2 cutover. blockchain.New rejects params that
// violate this.
func TestShippedNetworksRankPenaltyOrdering(t *testing.T) {
	for name, params := range map[string]*Params{
		"mainnet":  &MainNetParams,
		"testnet":  &TestNetParams,
		"testnet2": &TestNet2Params,
		"regtest":  &RegressionNetParams,
		"simnet":   &SimNetParams,
	} {
		if params.RankPenaltyForkHeight == 0 {
			continue
		}
		require.GreaterOrEqualf(t, params.RankPenaltyForkHeight, params.MoEForkHeight,
			"%s must not activate the rank-penalty fork before the MoE fork", name)
	}
}

// TestShippedNetworksForkHeights pins the proof-rule activation heights for
// the shipped networks so they cannot change accidentally.
//
// Lattice is a fresh chain with no pre-fork history to stay compatible with,
// so every network activates the newest proof rules at height 1: V3 salted-seed
// certificates, dense-only proofs, and the rank-penalty rule. There is no era
// of this chain in which the older, weaker formats were ever valid.
func TestShippedNetworksForkHeights(t *testing.T) {
	nets := map[string]*Params{
		"mainnet":  &MainNetParams,
		"regtest":  &RegressionNetParams,
		"testnet":  &TestNetParams,
		"testnet2": &TestNet2Params,
		"simnet":   &SimNetParams,
	}
	for name, p := range nets {
		require.Equalf(t, int32(1), p.MoEForkHeight,
			"%s must activate the MoE fork at height 1", name)
		require.Equalf(t, int32(1), p.SaltedSeedForkHeight,
			"%s must activate the salted-seed fork at height 1", name)
		require.Equalf(t, int32(1), p.DenseOnlyForkHeight,
			"%s must activate the dense-only fork at height 1", name)
		require.Equalf(t, int32(1), p.RankPenaltyForkHeight,
			"%s must activate the rank-penalty fork at height 1", name)

		// And the consequence those heights are there to produce.
		require.Equalf(t, wire.CertificateVersionV3, p.RequiredCertVersion(1),
			"%s must require V3 certificates from the first block", name)
	}
}

// TestShippedNetworksLatticeSeedForkHeights pins the heights at which each
// network leaves the proof-of-work domain it shares with Pearl.
//
// The shared domain is what lets a third-party miner for Pearl's algorithm
// produce valid Lattice blocks, which is the only consumer-GPU mining this
// chain has. Moving these heights changes when that stops working, so they are
// pinned rather than left to drift.
func TestShippedNetworksLatticeSeedForkHeights(t *testing.T) {
	want := map[string]struct {
		params *Params
		height int32
	}{
		// Six months of 40-second blocks.
		"mainnet": {&MainNetParams, 394200},
		// Twenty days, so the testnets rehearse the cutover first.
		"testnet":  {&TestNetParams, 43200},
		"testnet2": {&TestNet2Params, 43200},
		// Disabled: neither network verifies a proof by default.
		"regtest": {&RegressionNetParams, 0},
		"simnet":  {&SimNetParams, 0},
	}
	for name, tc := range want {
		p := tc.params
		require.Equalf(t, tc.height, p.LatticeSeedForkHeight,
			"%s Lattice-domain seed fork height", name)

		if tc.height == 0 {
			require.Falsef(t, p.IsLatticeSeedForkActive(1_000_000),
				"%s must never activate the Lattice-domain seed fork", name)
			continue
		}

		// V4 supersedes V3, so it must not activate first.
		require.GreaterOrEqualf(t, p.LatticeSeedForkHeight, p.SaltedSeedForkHeight,
			"%s must not activate the Lattice-domain fork before the salted-seed fork", name)

		// The cutover itself: V3 up to the height, V4 from it on.
		require.Equalf(t, wire.CertificateVersionV3, p.RequiredCertVersion(tc.height-1),
			"%s must still require V3 the block before the fork", name)
		require.Equalf(t, wire.CertificateVersionV4, p.RequiredCertVersion(tc.height),
			"%s must require V4 from the fork height on", name)
	}
}

// compactToBig is a copy of the blockchain.CompactToBig function. We copy it
// here so we don't run into a circular dependency just because of a test.
func compactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes to represent the full 256-bit number.  So,
	// treat the exponent as the number of bytes and shift the mantissa
	// right or left accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}

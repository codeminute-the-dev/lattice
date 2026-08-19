// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/chaincfg"
	"github.com/codeminute-the-dev/lattice/node/wire"
	"github.com/stretchr/testify/require"
)

// Tests for the Lattice-domain noise-seed (V4) certificate version cutover on
// SimNet, mirroring the V3 cutover tests in salted_seed_fork_test.go.
//
// The cutover is the point at which Lattice stops sharing Pearl's proof-of-work
// domain. Getting the height wrong in either direction is a chain split, so the
// strictness is worth pinning: V3 must be rejected at and after the height, and
// V4 must be rejected before it.

const latticeForkTestHeight = int32(4)

func latticeSimNetParams() chaincfg.Params {
	params := chaincfg.SimNetParams
	params.ReduceMinDifficulty = false
	params.MoEForkHeight = 1
	params.SaltedSeedForkHeight = 1
	params.LatticeSeedForkHeight = latticeForkTestHeight
	return params
}

func v4Cert(proofData []byte, publicDataLen uint32) *wire.CertificateV4 {
	cert := &wire.CertificateV4{}
	cert.ProofData = proofData
	cert.PublicDataLen = publicDataLen
	return cert
}

// TestLatticeSeedForkActivationAcceptsRequiredVersion builds a chain across the
// activation boundary and asserts each block is accepted with the version
// required at its height (V3 before the fork, V4 at and after it). Blocks come
// from SolveBlock, so mining-side version selection is covered too.
func TestLatticeSeedForkActivationAcceptsRequiredVersion(t *testing.T) {
	params := latticeSimNetParams()
	chain, teardown, err := chainSetup("lattice_fork_accept", &params)
	require.NoError(t, err)
	defer teardown()

	tip := btcutil.NewBlock(chain.chainParams.GenesisBlock)
	tip.SetHeight(0)

	for h := int32(1); h <= latticeForkTestHeight+2; h++ {
		block, _, err := addBlock(chain, tip, nil)
		require.NoErrorf(t, err, "block at height %d should be accepted", h)

		want := wire.CertificateVersionV3
		if h >= latticeForkTestHeight {
			want = wire.CertificateVersionV4
		}
		require.Equalf(t, want, block.MsgBlock().BlockCertificate().Version(),
			"unexpected certificate version at height %d", h)

		tip = block
	}

	require.Equal(t, latticeForkTestHeight+2, chain.BestSnapshot().Height)
}

// TestLatticeSeedForkRejectsWrongVersion asserts the strict cutover: a V4
// certificate is rejected below the activation height, and a V3 certificate is
// rejected at it.
func TestLatticeSeedForkRejectsWrongVersion(t *testing.T) {
	params := latticeSimNetParams()
	chain, teardown, err := chainSetup("lattice_fork_reject", &params)
	require.NoError(t, err)
	defer teardown()

	tip := btcutil.NewBlock(chain.chainParams.GenesisBlock)
	tip.SetHeight(0)

	// Extend until the next block is pre-fork (height latticeForkTestHeight-1).
	for h := int32(1); h <= latticeForkTestHeight-2; h++ {
		block, _, err := addBlock(chain, tip, nil)
		require.NoError(t, err)
		tip = block
	}

	bad := newBlockForcedCert(t, chain, tip, v4Cert([]byte{0x00}, 0))
	_, _, err = chain.ProcessBlock(bad, BFNone)
	requireRuleError(t, err, ErrDisallowedCertVersion)

	// Advance to the activation height with a valid block, then force V3.
	block, _, err := addBlock(chain, tip, nil)
	require.NoError(t, err)
	tip = block

	bad = newBlockForcedCert(t, chain, tip, v3Cert([]byte{0x00}, 0))
	_, _, err = chain.ProcessBlock(bad, BFNone)
	requireRuleError(t, err, ErrDisallowedCertVersion)

	// The tip must be unchanged by the rejected blocks.
	require.Equal(t, latticeForkTestHeight-1, chain.BestSnapshot().Height)
}

// TestCheckCertificateRulesDenseOnlyAppliesToV4 asserts the dense-only rule
// reads the V2 payload embedded two levels down in a V4 certificate.
func TestCheckCertificateRulesDenseOnlyAppliesToV4(t *testing.T) {
	params := &chaincfg.Params{
		MoEForkHeight:         1,
		SaltedSeedForkHeight:  1,
		LatticeSeedForkHeight: 2,
		DenseOnlyForkHeight:   2,
	}

	dense := v4Cert(nil, wire.PublicDataSizeDenseV2)
	moe := v4Cert(nil, wire.PublicDataSizeDenseV2+1)
	placeholder := v4Cert(nil, 0)

	require.NoError(t, checkCertVersion(dense, 2, params))
	require.NoError(t, checkCertVersion(placeholder, 2, params))
	requireRuleError(t, checkCertVersion(moe, 2, params),
		ErrDisallowedCertVersion)
}

// TestLatticeSeedForkBlockTemplateVersion verifies CheckConnectBlockTemplate
// enforces the required cert version on unmined templates (BFNoPoWCheck) at the
// fork height.
func TestLatticeSeedForkBlockTemplateVersion(t *testing.T) {
	params := latticeSimNetParams()
	chain, teardown, err := chainSetup("lattice_fork_template", &params)
	require.NoError(t, err)
	defer teardown()

	tip := btcutil.NewBlock(chain.chainParams.GenesisBlock)
	tip.SetHeight(0)

	// Advance until the next block is at the activation height.
	for h := int32(1); h < latticeForkTestHeight; h++ {
		block, _, err := addBlock(chain, tip, nil)
		require.NoError(t, err)
		tip = block
	}

	// At the fork height, a template carrying a V4 certificate must connect.
	v4Template := newBlockForcedCert(t, chain, tip,
		&wire.CertificateV4{CertificateV3: wire.CertificateV3{
			CertificateV2: wire.CertificateV2{Hash: *tip.Hash()},
		}})
	require.Equal(t, wire.CertificateVersionV4,
		v4Template.MsgBlock().BlockCertificate().Version())
	require.NoError(t, chain.CheckConnectBlockTemplate(v4Template),
		"V4 template must be accepted at the fork height")

	// The same template carrying a V3 certificate must be rejected.
	v3Template := newBlockForcedCert(t, chain, tip,
		&wire.CertificateV3{CertificateV2: wire.CertificateV2{Hash: *tip.Hash()}})
	requireRuleError(t, chain.CheckConnectBlockTemplate(v3Template),
		ErrDisallowedCertVersion)
}

// TestLatticeSeedForkOrderingRejected asserts blockchain.New refuses params
// that would put the V4 cutover before the V3 one, or enable V4 with no V3
// cutover underneath it — either would leave no height at which V3 applies.
func TestLatticeSeedForkOrderingRejected(t *testing.T) {
	for name, mutate := range map[string]func(p *chaincfg.Params){
		"before salted fork": func(p *chaincfg.Params) {
			p.SaltedSeedForkHeight = 10
			p.LatticeSeedForkHeight = 9
		},
		"salted fork disabled": func(p *chaincfg.Params) {
			p.SaltedSeedForkHeight = 0
			p.LatticeSeedForkHeight = 10
		},
		"negative height": func(p *chaincfg.Params) {
			p.LatticeSeedForkHeight = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			params := latticeSimNetParams()
			mutate(&params)
			_, teardown, err := chainSetup("lattice_fork_ordering", &params)
			if teardown != nil {
				defer teardown()
			}
			require.Error(t, err)
		})
	}
}

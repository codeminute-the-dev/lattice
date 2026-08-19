// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"github.com/codeminute-the-dev/lattice/node/chaincfg/chainhash"
)

// CertificateV4 is a version-4 (V4) block certificate: wire layout identical
// to CertificateV3, but the verifier salts the Merkle roots under Lattice's
// own noise-seed domain instead of the inherited Pearl one.
//
// V3 exists so Lattice's proof of work is bit-identical to Pearl's, which lets
// an existing third-party miner produce valid Lattice work. V4 is where the
// chain leaves that shared domain; see LatticeSeedForkHeight in chaincfg.
type CertificateV4 struct {
	CertificateV3
}

func (c *CertificateV4) Version() CertificateVersion {
	return CertificateVersionV4
}

// ProofCommitment hashes version 4, not the embedded V3's version.
func (c *CertificateV4) ProofCommitment() chainhash.Hash {
	return proofCommitment(c.Version(), c.PublicDataBytes())
}

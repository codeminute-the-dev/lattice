// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"testing"
)

func allNets() map[string]*Params {
	return map[string]*Params{
		"mainnet":  &MainNetParams,
		"regtest":  &RegressionNetParams,
		"testnet":  &TestNetParams,
		"testnet2": &TestNet2Params,
		"simnet":   &SimNetParams,
	}
}

// TestGenesisIsReproducible re-derives every genesis hash from the block it
// claims to describe. genesis.go is generated, and a generated file can go
// stale if someone hand-edits a literal; this fails the build rather than
// letting a node ship with a genesis hash that does not match its own block.
func TestGenesisIsReproducible(t *testing.T) {
	for name, p := range allNets() {
		t.Run(name, func(t *testing.T) {
			computed := p.GenesisBlock.BlockHash()
			if !p.GenesisHash.IsEqual(&computed) {
				t.Fatalf("declared genesis hash %s does not match the block's own hash %s",
					p.GenesisHash, computed)
			}
			// The merkle root must be the coinbase transaction's hash,
			// since genesis holds exactly one transaction.
			if len(p.GenesisBlock.Transactions) != 1 {
				t.Fatalf("genesis holds %d transactions, want exactly 1",
					len(p.GenesisBlock.Transactions))
			}
			txHash := p.GenesisBlock.Transactions[0].TxHash()
			if !p.GenesisBlock.BlockHeader().MerkleRoot.IsEqual(&txHash) {
				t.Fatalf("merkle root %s is not the coinbase hash %s",
					p.GenesisBlock.BlockHeader().MerkleRoot, txHash)
			}
		})
	}
}

// TestGenesisHasNoPremine is the fair-launch guarantee expressed as a test.
//
// Lattice's legitimacy rests on the claim that no coins existed before the
// first mined block. That claim is only as good as the genesis block, so this
// asserts it directly: every genesis coinbase output must carry zero value and
// be provably unspendable (a bare OP_RETURN), on every network.
func TestGenesisHasNoPremine(t *testing.T) {
	for name, p := range allNets() {
		t.Run(name, func(t *testing.T) {
			var total int64
			for i, out := range p.GenesisBlock.Transactions[0].TxOut {
				total += out.Value
				if out.Value != 0 {
					t.Errorf("genesis output %d allocates %d cells; a fair launch allocates none",
						i, out.Value)
				}
				if len(out.PkScript) != 1 || out.PkScript[0] != 0x6a {
					t.Errorf("genesis output %d script is %x, want a bare OP_RETURN (6a)",
						i, out.PkScript)
				}
			}
			if total != 0 {
				t.Fatalf("genesis creates %d cells, want 0", total)
			}
		})
	}
}

// TestGenesisHashesAreDistinct keeps the networks from sharing a genesis hash,
// which would let peers on different networks mistake each other for their own.
func TestGenesisHashesAreDistinct(t *testing.T) {
	seen := make(map[string]string)
	for name, p := range allNets() {
		h := p.GenesisHash.String()
		if prev, ok := seen[h]; ok {
			t.Errorf("%s and %s share genesis hash %s", prev, name, h)
		}
		seen[h] = name
	}
}

// TestGenesisCommitsToLaunchParameters checks the coinbase message still names
// the emission constants it advertises. If the schedule is retuned without
// updating the genesis message, the chain's own first block would describe
// parameters it does not follow.
func TestGenesisCommitsToLaunchParameters(t *testing.T) {
	script := string(genesisCoinbaseTx.TxIn[0].SignatureScript)
	for _, want := range []string{
		"No premine",
		"Reward 2 LATT",
		"39420000", // the emission constant k
		"3153600",  // the epoch length in blocks
		"Bitcoin block #963094",
	} {
		if !contains(script, want) {
			t.Errorf("genesis coinbase message does not mention %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestEmissionParamsAreWired guards against a network shipping without an
// emission schedule, which would silently fall back to mainnet's.
func TestEmissionParamsAreWired(t *testing.T) {
	for name, p := range allNets() {
		t.Run(name, func(t *testing.T) {
			e := p.Emission
			if e.EpochBlocks <= 0 || e.Constant <= 0 || e.ReferenceSupply <= 0 || e.ResetThreshold <= 0 {
				t.Fatalf("incomplete emission params: %+v", e)
			}
			if got := e.InitialSubsidy(); got != 2*1e8 {
				t.Errorf("initial subsidy %d cells, want 2 LATT", got)
			}
		})
	}
}

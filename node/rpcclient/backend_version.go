package rpcclient

import "fmt"

// BackendVersion defines an interface to handle the version of the backend
// used by the client.
type BackendVersion interface {
	// String returns a human-readable backend version.
	String() string

	// SupportTestMempoolAccept returns true if the backend supports the
	// testmempoolaccept RPC.
	SupportTestMempoolAccept() bool

	// SupportGetTxSpendingPrevOut returns true if the backend supports the
	// gettxspendingprevout RPC.
	SupportGetTxSpendingPrevOut() bool
}

// CompatForkVersion represents a protocol-compatible fork backend.
// Compatible forks refer to Bitcoin Core-based forks that implement the Lattice protocol.
// All modern RPCs are assumed to be supported.
type CompatForkVersion struct{}

func (CompatForkVersion) String() string                    { return "compatible fork" }
func (CompatForkVersion) SupportTestMempoolAccept() bool    { return true }
func (CompatForkVersion) SupportGetTxSpendingPrevOut() bool { return true }

var _ BackendVersion = CompatForkVersion{}

// LatticedVersion represents the latticed node backend as the numeric version
// reported by GetInfo, enabling future feature-gating as the protocol evolves.
// Currently all RPCs are supported since there are no legacy latticed versions
// in the wild.
type LatticedVersion int32

// To gate features on a minimum latticed version, define a const here using the
// numeric format from GetInfo (1000000*major + 10000*minor + 100*patch) and
// compare against it in the Support* methods. For example:
//
//	const LatticedSomeFeature LatticedVersion = 260000 // v0.26.0
//
//	func (v LatticedVersion) SupportSomeFeature() bool { return v >= LatticedSomeFeature }

func (v LatticedVersion) String() string {
	return fmt.Sprintf("latticed %d", v)
}

func (LatticedVersion) SupportTestMempoolAccept() bool    { return true }
func (LatticedVersion) SupportGetTxSpendingPrevOut() bool { return true }

var _ BackendVersion = LatticedVersion(0)

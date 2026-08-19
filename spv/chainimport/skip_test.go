package chainimport

import (
	"fmt"
	"os"
	"testing"
)

// TODO Or: update when Lattice mainnet header test data is available.
// The chainimport tests use Bitcoin mainnet header dumps (80-byte headers with
// Nonce) which are incompatible with Lattice's wire format (108-byte headers with
// ProofCommitment and block certificates). Once Lattice mainnet has enough blocks,
// generate Lattice-specific test data and re-enable these tests.
func TestMain(m *testing.M) {
	fmt.Println("chainimport: skipping tests (needs Lattice mainnet header test data)")
	os.Exit(0)
}

// Command genaddr derives a deterministic Taproot address for a given network,
// for local testing and dogfooding only. It is not a wallet: the key is fixed
// and public, so never send real funds to what it prints.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/codeminute-the-dev/lattice/node/btcec"
	"github.com/codeminute-the-dev/lattice/node/btcec/schnorr"
	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/chaincfg"
	"github.com/codeminute-the-dev/lattice/node/txscript"
)

func main() {
	netName := "simnet"
	if len(os.Args) > 1 {
		netName = os.Args[1]
	}
	nets := map[string]*chaincfg.Params{
		"mainnet": &chaincfg.MainNetParams,
		"testnet": &chaincfg.TestNetParams,
		"simnet":  &chaincfg.SimNetParams,
		"regtest": &chaincfg.RegressionNetParams,
	}
	p, ok := nets[netName]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown network %q\n", netName)
		os.Exit(1)
	}

	seed, _ := hex.DecodeString(
		"5db1fee4b5a3f0dbf9c2eb0b0e4a6b8c1d2e3f405162738495a6b7c8d9e0f102")
	priv, _ := btcec.PrivKeyFromBytes(seed)
	taprootKey := txscript.ComputeTaprootKeyNoScript(priv.PubKey())

	addr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(taprootKey), p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(addr.EncodeAddress())
}

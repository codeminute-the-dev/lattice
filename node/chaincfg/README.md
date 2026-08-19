chaincfg
========

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/chaincfg)

Package chaincfg defines chain configuration parameters for the three standard
Lattice networks and provides the ability for callers to define their own custom
Lattice networks.

Although this package was primarily written for latticed, it has intentionally been
designed so it can be used as a standalone package for any projects needing to
use parameters for the standard Lattice networks or for projects needing to
define their own network.

## Sample Use

```Go
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/chaincfg"
)

var testnet = flag.Bool("testnet", false, "operate on the testnet Lattice network")

// By default (without -testnet), use mainnet.
var chainParams = &chaincfg.MainNetParams

func main() {
	flag.Parse()

	// Modify active network parameters if operating on testnet.
	if *testnet {
		chainParams = &chaincfg.TestNetParams
	}

	// later...

	// Create and print new payment address, specific to the active network.
	pubKeyHash := make([]byte, 20)
	addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, chainParams)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(addr)
}
```

## Installation and Updating

This package is part of the `github.com/codeminute-the-dev/lattice` module. Use it
as a dependency in your Go module:

```bash
$ go get github.com/codeminute-the-dev/lattice/node/chaincfg
```

## License

Package chaincfg is licensed under the [copyfree](http://copyfree.org) ISC
License.

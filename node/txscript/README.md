txscript
========

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/txscript?status.png)](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/txscript)

Package txscript implements the Lattice transaction script language.  There is
a comprehensive test suite.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to use or validate Lattice transaction scripts.

## Lattice Scripts

Lattice provides a stack-based, FORTH-like language for the scripts in
the Lattice transactions.  This language is not turing complete
although it is still fairly powerful.  A description of the language
can be found at https://en.bitcoin.it/wiki/Script

## Installation and Updating

This package is part of the `github.com/codeminute-the-dev/lattice` module. Use it
as a dependency in your Go module:

```bash
$ go get github.com/codeminute-the-dev/lattice/node/txscript
```

## Examples

* [Standard Pay-to-pubkey-hash Script](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/txscript#example-PayToAddrScript)  
  Demonstrates creating a script which pays to a Lattice address.  It also
  prints the created script hex and uses the DisasmString function to display
  the disassembled script.

* [Extracting Details from Standard Scripts](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/txscript#example-ExtractPkScriptAddrs)  
  Demonstrates extracting information from a standard public key script.

* [Manually Signing a Transaction Output](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/txscript#example-SignTxOutput)  
  Demonstrates manually creating and signing a redeem transaction.

* [Counting Opcodes in Scripts](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/txscript#example-ScriptTokenizer)  
  Demonstrates creating a script tokenizer instance and using it to count the
  number of opcodes a script contains.

## License

Package txscript is licensed under the [copyfree](http://copyfree.org) ISC
License.

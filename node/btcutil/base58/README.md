base58
==========

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/codeminute-the-dev/lattice/node/btcutil/base58)

Package base58 provides an API for encoding and decoding to and from the
modified base58 encoding.  It also provides an API to do Base58Check encoding,
as described [here](https://en.bitcoin.it/wiki/Base58Check_encoding).

A comprehensive suite of tests is provided to ensure proper functionality.

## Installation

This package is part of the `github.com/codeminute-the-dev/lattice` module. Use it as a dependency in your Go project:

```bash
go get github.com/codeminute-the-dev/lattice
```

## Examples

* [Decode Example](http://godoc.org/github.com/codeminute-the-dev/lattice/node/btcutil/base58#example-Decode)  
  Demonstrates how to decode modified base58 encoded data.
* [Encode Example](http://godoc.org/github.com/codeminute-the-dev/lattice/node/btcutil/base58#example-Encode)  
  Demonstrates how to encode data using the modified base58 encoding scheme.
* [CheckDecode Example](http://godoc.org/github.com/codeminute-the-dev/lattice/node/btcutil/base58#example-CheckDecode)  
  Demonstrates how to decode Base58Check encoded data.
* [CheckEncode Example](http://godoc.org/github.com/codeminute-the-dev/lattice/node/btcutil/base58#example-CheckEncode)  
  Demonstrates how to encode data using the Base58Check encoding scheme.

## License

Package base58 is licensed under the [copyfree](http://copyfree.org) ISC
License.

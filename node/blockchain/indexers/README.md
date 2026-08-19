indexers
========

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/blockchain/indexers?status.png)](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/blockchain/indexers)

Package indexers implements optional block chain indexes.

These indexes are typically used to enhance the amount of information available
via an RPC interface.

## Supported Indexers

- Transaction-by-hash (txbyhashidx) Index
  - Creates a mapping from the hash of each transaction to the block that
    contains it along with its offset and length within the serialized block
- Transaction-by-address (txbyaddridx) Index
  - Creates a mapping from every address to all transactions which either credit
    or debit the address
  - Requires the transaction-by-hash index

## Installation

This package is part of the `github.com/codeminute-the-dev/lattice` module. Use it as a dependency in your Go project:

```bash
go get github.com/codeminute-the-dev/lattice
```

## License

Package indexers is licensed under the [copyfree](http://copyfree.org) ISC
License.

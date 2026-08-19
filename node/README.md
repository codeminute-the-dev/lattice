# latticed

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node)

latticed is the reference implementation of the Lattice Protocol. It is a full node
that downloads, validates, and serves the Lattice blockchain. latticed includes
zero-knowledge proof-of-work verification and XMSS post-quantum signature
support.

latticed properly relays newly mined blocks, maintains a transaction pool, and
relays individual transactions that have not yet made it into a block. It
ensures all transactions admitted to the pool follow the consensus rules and
also includes stricter checks which filter transactions based on miner
requirements ("standard" transactions).

latticed does *not* include wallet functionality. That is provided by the
[Latwallet wallet](https://github.com/codeminute-the-dev/lattice/tree/master/wallet).

## Requirements

- [Go](https://golang.org) 1.26 or newer
- [Rust](https://rustup.rs) toolchain (for ZK verification library)
- C compiler (for XMSS library)
- [Task](https://taskfile.dev) runner

## Building

From the repository root:

```bash
task build:latticed
```

Or to build all binaries (latticed, latctl, latwallet):

```bash
task build:blockchain
```

Binaries are placed in `bin/`.

## Getting Started

latticed has several configuration options available to tweak how it runs, but all
of the basic operations work with zero configuration.

```bash
./bin/latticed
```

See [sample-latticed.conf](sample-latticed.conf) for the full list of options.

## Documentation

Documentation is located in the [docs](docs/) folder.

## Issue Tracker

The [integrated GitHub issue tracker](https://github.com/codeminute-the-dev/lattice/issues)
is used for this project.

## License

latticed is licensed under the [copyfree](http://copyfree.org) ISC License.

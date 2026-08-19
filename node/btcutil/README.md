btcutil
=======

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/codeminute-the-dev/lattice/node/btcutil)

Package btcutil provides Lattice-specific convenience functions and types.
A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.

This package was developed for latticed, an alternative full-node implementation of
Lattice.  Although it was primarily written for latticed, this package has
intentionally been designed so it can be used as a standalone package for any
projects needing the functionality provided.

## Installation

This package is part of the `github.com/codeminute-the-dev/lattice` module. Use it
by adding the module as a dependency in your project.

## License

Package btcutil is licensed under the [copyfree](http://copyfree.org) ISC
License.

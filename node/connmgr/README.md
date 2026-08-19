connmgr
=======

[![Build Status](https://github.com/codeminute-the-dev/lattice/workflows/Build%20and%20Test/badge.svg)](https://github.com/codeminute-the-dev/lattice/actions)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/connmgr)

Package connmgr implements a generic Lattice network connection manager.

## Overview

Connection Manager handles all the general connection concerns such as
maintaining a set number of outbound connections, sourcing peers, banning,
limiting max connections, tor lookup, etc.

The package provides a generic connection manager which is able to accept
connection requests from a source or a set of given addresses, dial them and
notify the caller on connections. The main intended use is to initialize a pool
of active connections and maintain them to remain connected to the P2P network.

In addition the connection manager provides the following utilities:

- Notifications on connections or disconnections
- Handle failures and retry new addresses from the source
- Connect only to specified addresses
- Permanent connections with increasing backoff retry timers
- Disconnect or Remove an established connection

## Installation

This package is part of the `github.com/codeminute-the-dev/lattice` module. Use it as a dependency in your Go project:

```bash
go get github.com/codeminute-the-dev/lattice
```

## License

Package connmgr is licensed under the [copyfree](http://copyfree.org) ISC License.

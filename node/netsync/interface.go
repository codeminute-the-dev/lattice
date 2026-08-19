// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"github.com/codeminute-the-dev/lattice/node/blockchain"
	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/chaincfg"
	"github.com/codeminute-the-dev/lattice/node/chaincfg/chainhash"
	"github.com/codeminute-the-dev/lattice/node/mempool"
	"github.com/codeminute-the-dev/lattice/node/peer"
	"github.com/codeminute-the-dev/lattice/node/wire"
)

// PeerNotifier exposes methods to notify peers of status changes to
// transactions, blocks, etc. Currently server (in the main package) implements
// this interface.
type PeerNotifier interface {
	AnnounceNewTransactions(newTxs []*mempool.TxDesc)

	UpdatePeerHeights(latestBlkHash *chainhash.Hash, latestHeight int32, updateSource *peer.Peer)

	RelayInventory(invVect *wire.InvVect, data interface{})

	TransactionConfirmed(tx *btcutil.Tx)
}

// Config is a configuration struct used to initialize a new SyncManager.
type Config struct {
	PeerNotifier PeerNotifier
	Chain        *blockchain.BlockChain
	TxMemPool    *mempool.TxPool
	ChainParams  *chaincfg.Params

	DisableCheckpoints bool
	MaxPeers           int

	FeeEstimator *mempool.FeeEstimator
}

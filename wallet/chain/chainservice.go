package chain

import (
	"context"

	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/btcutil/gcs"
	"github.com/codeminute-the-dev/lattice/node/chaincfg"
	"github.com/codeminute-the-dev/lattice/node/chaincfg/chainhash"
	"github.com/codeminute-the-dev/lattice/node/wire"
	neutrino "github.com/codeminute-the-dev/lattice/spv"
	"github.com/codeminute-the-dev/lattice/spv/banman"
	"github.com/codeminute-the-dev/lattice/spv/headerfs"
)

// NeutrinoChainService is an interface that encapsulates all the public
// methods of a *neutrino.ChainService
type NeutrinoChainService interface {
	Start(ctx context.Context) error
	GetBlock(chainhash.Hash, ...neutrino.QueryOption) (*btcutil.Block, error)
	GetBlockHeight(*chainhash.Hash) (int32, error)
	BestBlock() (*headerfs.BlockStamp, error)
	BlockHeaderTipHeight() (int32, error)
	FilterHeaderTipHeight() (int32, error)
	BestPeerHeight() int32
	GetBlockHash(int64) (*chainhash.Hash, error)
	GetBlockHeader(*chainhash.Hash) (*wire.BlockHeader, error)
	IsCurrent() bool
	SendTransaction(*wire.MsgTx) error
	GetCFilter(chainhash.Hash, wire.FilterType,
		...neutrino.QueryOption) (*gcs.Filter, error)
	GetUtxo(...neutrino.RescanOption) (*neutrino.SpendReport, error)
	BanPeer(string, banman.Reason) error
	IsBanned(addr string) bool
	AddPeer(*neutrino.ServerPeer)
	AddBytesSent(uint64)
	AddBytesReceived(uint64)
	NetTotals() (uint64, uint64)
	UpdatePeerHeights(*chainhash.Hash, int32, *neutrino.ServerPeer)
	ChainParams() chaincfg.Params
	Stop() error
	PeerByAddr(string) *neutrino.ServerPeer
}

var _ NeutrinoChainService = (*neutrino.ChainService)(nil)

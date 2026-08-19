//go:build zkpow

// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/codeminute-the-dev/lattice/node/blockchain"
	"github.com/codeminute-the-dev/lattice/node/btcutil"
	"github.com/codeminute-the-dev/lattice/node/chaincfg/chainhash"
	"github.com/codeminute-the-dev/lattice/node/wire"
)

// nodeClient is the proxy's link to latticed. Two calls do everything: fetch a
// template to hand out as work, and submit the one block that comes back.
type nodeClient struct {
	addr string
	user string
	pass string
	http *http.Client
}

func newNodeClient(addr, user, pass string) *nodeClient {
	return &nodeClient{addr: addr, user: user, pass: pass,
		http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *nodeClient) call(method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "1.0", "id": "latstratum", "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "http://"+c.addr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&env); err != nil {
		return nil, fmt.Errorf("unparseable reply from node: %w", err)
	}
	if env.Error != nil {
		return nil, fmt.Errorf("%s", env.Error.Message)
	}
	return env.Result, nil
}

// blockTemplate is the subset of getblocktemplate the proxy uses.
//
// The coinbasetxn capability is what keeps this simple: the node builds the
// coinbase, paying whatever address it was configured with, so the proxy never
// constructs one. That also means the payout address is the node's business,
// not a field a miner can influence.
type blockTemplate struct {
	Version             int32  `json:"version"`
	PreviousBlockHash   string `json:"previousblockhash"`
	CurTime             int64  `json:"curtime"`
	Bits                string `json:"bits"`
	Height              int32  `json:"height"`
	Target              string `json:"target"`
	RequiredCertVersion uint32 `json:"requiredcertversion"`
	CoinbaseTxn         *struct {
		Data string `json:"data"`
	} `json:"coinbasetxn"`
	Transactions []struct {
		Data string `json:"data"`
	} `json:"transactions"`
}

func (c *nodeClient) getBlockTemplate() (*blockTemplate, error) {
	raw, err := c.call("getblocktemplate", map[string]any{
		"capabilities": []string{"coinbasetxn"},
		"rules":        []string{"segwit"},
	})
	if err != nil {
		return nil, err
	}
	var t blockTemplate
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	if t.CoinbaseTxn == nil {
		return nil, fmt.Errorf("node returned no coinbasetxn; it must be built with a mining address")
	}
	return &t, nil
}

func (c *nodeClient) submitBlock(blk *wire.MsgBlock) error {
	var buf bytes.Buffer
	if err := blk.Serialize(&buf); err != nil {
		return err
	}
	raw, err := c.call("submitblock", hex.EncodeToString(buf.Bytes()))
	if err != nil {
		return err
	}
	// submitblock answers null on success and a rejection reason otherwise.
	var reason *string
	if err := json.Unmarshal(raw, &reason); err == nil && reason != nil {
		return fmt.Errorf("node rejected block: %s", *reason)
	}
	return nil
}

// job is one unit of work: the 76-byte incomplete header a miner searches,
// plus everything needed to rebuild the full block if it finds one.
type job struct {
	ID          string
	Height      int32
	CertVersion wire.CertificateVersion
	Header      wire.BlockHeader
	HeaderHex   string
	ShareTarget string
	Txns        []*btcutil.Tx // coinbase first
	CreatedAt   time.Time
}

// buildJob turns a template into work.
//
// The header a miner searches is "incomplete": everything except the proof
// commitment, which only exists once the proof does. Serialized, that is
// version | prev_block | merkle_root | timestamp | nbits, 76 bytes, with the
// two hashes in display order — the same layout the FFI's IncompleteBlockHeader
// uses and the same one the reference pool puts on the wire.
func buildJob(t *blockTemplate, id string, shareTarget string) (*job, error) {
	prev, err := chainhash.NewHashFromStr(t.PreviousBlockHash)
	if err != nil {
		return nil, fmt.Errorf("bad previousblockhash: %w", err)
	}
	bits64, err := strconv.ParseUint(t.Bits, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("bad bits: %w", err)
	}

	txns := make([]*btcutil.Tx, 0, len(t.Transactions)+1)
	cbBytes, err := hex.DecodeString(t.CoinbaseTxn.Data)
	if err != nil {
		return nil, fmt.Errorf("bad coinbasetxn: %w", err)
	}
	cb, err := btcutil.NewTxFromBytes(cbBytes)
	if err != nil {
		return nil, fmt.Errorf("undecodable coinbasetxn: %w", err)
	}
	txns = append(txns, cb)
	for i, raw := range t.Transactions {
		b, err := hex.DecodeString(raw.Data)
		if err != nil {
			return nil, fmt.Errorf("bad transaction %d: %w", i, err)
		}
		tx, err := btcutil.NewTxFromBytes(b)
		if err != nil {
			return nil, fmt.Errorf("undecodable transaction %d: %w", i, err)
		}
		txns = append(txns, tx)
	}

	hdr := wire.BlockHeader{
		Version:    t.Version,
		PrevBlock:  *prev,
		MerkleRoot: blockchain.CalcMerkleRoot(txns, false),
		Timestamp:  time.Unix(t.CurTime, 0),
		Bits:       uint32(bits64),
	}

	return &job{
		ID:          id,
		Height:      t.Height,
		CertVersion: wire.CertificateVersion(t.RequiredCertVersion),
		Header:      hdr,
		HeaderHex:   incompleteHeaderHex(&hdr),
		ShareTarget: shareTarget,
		Txns:        txns,
		CreatedAt:   time.Now(),
	}, nil
}

// incompleteHeaderHex serializes the 76 bytes a miner searches.
func incompleteHeaderHex(h *wire.BlockHeader) string {
	buf := make([]byte, 0, 76)
	buf = append(buf,
		byte(h.Version), byte(h.Version>>8), byte(h.Version>>16), byte(h.Version>>24))
	// Display order, matching IncompleteBlockHeader in the FFI: the wire keeps
	// hashes little-endian, the struct keeps them reversed.
	for i := len(h.PrevBlock) - 1; i >= 0; i-- {
		buf = append(buf, h.PrevBlock[i])
	}
	for i := len(h.MerkleRoot) - 1; i >= 0; i-- {
		buf = append(buf, h.MerkleRoot[i])
	}
	ts := uint32(h.Timestamp.Unix())
	buf = append(buf, byte(ts), byte(ts>>8), byte(ts>>16), byte(ts>>24))
	buf = append(buf, byte(h.Bits), byte(h.Bits>>8), byte(h.Bits>>16), byte(h.Bits>>24))
	return hex.EncodeToString(buf)
}

// assembleBlock rebuilds the full block around a certificate the pool proved.
func (j *job) assembleBlock(cert wire.BlockCertificate, header wire.BlockHeader) *wire.MsgBlock {
	blk := &wire.MsgBlock{
		MsgHeader: wire.MsgHeader{
			BlockHeader:    header,
			MsgCertificate: wire.MsgCertificate{Certificate: cert},
		},
	}
	for _, tx := range j.Txns {
		blk.Transactions = append(blk.Transactions, tx.MsgTx())
	}
	return blk
}

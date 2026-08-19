//go:build zkpow

// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command latstratum serves Lattice work over the stratum dialect SRBMiner
// speaks for `pearlhash`, so a consumer GPU can mine this chain.
//
// # Why this exists
//
// The reference miner needs H100-class hardware. The only implementation of
// this proof of work that runs on a consumer GPU is a third-party one, and it
// talks to a pool rather than to a node. Lattice's V3 work function is
// deliberately bit-identical to Pearl's, so that miner produces valid Lattice
// work without knowing Lattice exists — provided something speaks its protocol.
// That is this program's entire job.
//
// # Solo, not a pool
//
// There is no share ledger and no payout accounting. The node's coinbase pays
// whatever address latticed was configured with, so every block found pays the
// node operator. Shares exist only to prove a miner is working and to keep the
// difficulty conversation going. Adding real pool accounting means adding a
// ledger; it does not mean changing anything here.
//
// # The two difficulties
//
// A miner is given an easy share target so it reports in often. Consensus wants
// the header's own nbits. Every accepted share is checked twice: once against
// the share target to count it, and once against the block target to see if it
// is the one worth proving. Only the second one costs anything, because turning
// a share into a certificate runs plonky2.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codeminute-the-dev/lattice/node/blockchain"
	"github.com/codeminute-the-dev/lattice/node/wire"
	"github.com/codeminute-the-dev/lattice/node/zkpow"
)

var (
	listenAddr = flag.String("listen", "0.0.0.0:3333", "stratum listen address")
	nodeAddr   = flag.String("node", "127.0.0.1:44107", "latticed RPC address")
	confPath   = flag.String("conf", "", "read rpcuser/rpcpass from this latticed.conf")
	rpcUser    = flag.String("rpcuser", "", "node RPC username")
	rpcPass    = flag.String("rpcpass", "", "node RPC password")
	shareBits  = flag.String("sharebits", "1e7fffff", "share difficulty as compact nbits (easier than the block target)")
	pollEvery  = flag.Duration("poll", 5*time.Second, "how often to refresh the block template")
	logShares  = flag.Bool("logshares", false, "log every submitted frame verbatim (noisy; for protocol work)")
)

func loadConf(path string) (user, pass string, err error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(f), "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, "="); ok {
			switch strings.TrimSpace(k) {
			case "rpcuser":
				user = strings.TrimSpace(v)
			case "rpcpass":
				pass = strings.TrimSpace(v)
			}
		}
	}
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("%s contains no rpcuser/rpcpass", path)
	}
	return user, pass, nil
}

// targetHex renders a compact-bits difficulty as the 32-byte big-endian hex
// string the protocol puts in a job.
func targetHex(bits uint32) string {
	t := blockchain.CompactToBig(bits)
	b := t.Bytes()
	if len(b) > 32 {
		b = b[len(b)-32:]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return fmt.Sprintf("%x", out)
}

// server holds the current job and the connected miners.
type server struct {
	node   *nodeClient
	shareB uint32

	mu      sync.RWMutex
	current *job
	clients map[*client]struct{}

	jobSeq   atomic.Uint64
	accepted atomic.Uint64
	rejected atomic.Uint64
	blocks   atomic.Uint64
}

type client struct {
	conn   net.Conn
	enc    *json.Encoder
	mu     sync.Mutex
	worker string
}

func (c *client) send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(v)
}

func (s *server) addClient(c *client) {
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
}

func (s *server) removeClient(c *client) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}

func (s *server) job() *job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// notify is the job push. The parameters are an object, not the positional
// array classic stratum uses — this dialect announces itself as "v2" and has no
// mining.subscribe at all.
func (s *server) notifyMsg(j *job) map[string]any {
	return map[string]any{
		"id":     nil,
		"method": "mining.notify",
		"params": map[string]any{
			"job_id":       j.ID,
			"header":       j.HeaderHex,
			"height":       j.Height,
			"cert_version": uint32(j.CertVersion),
			"target":       j.ShareTarget,
		},
	}
}

func (s *server) broadcast(j *job) {
	msg := s.notifyMsg(j)
	s.mu.RLock()
	cs := make([]*client, 0, len(s.clients))
	for c := range s.clients {
		cs = append(cs, c)
	}
	s.mu.RUnlock()
	for _, c := range cs {
		if err := c.send(msg); err != nil {
			c.conn.Close()
		}
	}
}

// refresh pulls a template and publishes it as a job when the chain has moved.
func (s *server) refresh() error {
	t, err := s.node.getBlockTemplate()
	if err != nil {
		return err
	}
	prev := s.job()
	if prev != nil && prev.Height == t.Height &&
		prev.Header.PrevBlock.String() == t.PreviousBlockHash {
		return nil // same tip; nothing to announce
	}

	id := fmt.Sprintf("%08x", s.jobSeq.Add(1))
	j, err := buildJob(t, id, targetHex(s.shareB))
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.current = j
	s.mu.Unlock()

	log.Printf("job %s: height %d, cert v%d, block bits %08x",
		j.ID, j.Height, j.CertVersion, j.Header.Bits)
	s.broadcast(j)
	return nil
}

func (s *server) poll() {
	for {
		if err := s.refresh(); err != nil {
			log.Printf("template refresh failed: %v", err)
		}
		time.Sleep(*pollEvery)
	}
}

func main() {
	flag.Parse()

	if *confPath != "" {
		u, p, err := loadConf(*confPath)
		if err != nil {
			log.Fatal(err)
		}
		*rpcUser, *rpcPass = u, p
	}
	if *rpcUser == "" || *rpcPass == "" {
		log.Fatal("need -conf, or -rpcuser and -rpcpass")
	}

	var sb uint32
	if _, err := fmt.Sscanf(*shareBits, "%x", &sb); err != nil {
		log.Fatalf("bad -sharebits %q: %v", *shareBits, err)
	}

	s := &server{
		node:    newNodeClient(*nodeAddr, *rpcUser, *rpcPass),
		shareB:  sb,
		clients: map[*client]struct{}{},
	}

	if err := s.refresh(); err != nil {
		log.Fatalf("could not get a first template from the node: %v", err)
	}
	go s.poll()

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("latstratum on %s, share target %s", *listenAddr, targetHex(sb))
	log.Printf("point a miner at it:  --algorithm pearlhash --pool %s --wallet <anything>", *listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go s.handle(conn)
	}
}

func (s *server) handle(conn net.Conn) {
	defer conn.Close()
	c := &client{conn: conn, enc: json.NewEncoder(conn)}
	s.addClient(c)
	defer s.removeClient(c)

	log.Printf("miner connected: %s", conn.RemoteAddr())
	defer log.Printf("miner gone: %s", conn.RemoteAddr())

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), zkpow.MaxPlainProofSize+1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if *logShares {
			log.Printf("<- %s", truncate(string(line), 4000))
		}
		var msg struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("unparseable frame from %s: %v", conn.RemoteAddr(), err)
			continue
		}

		switch msg.Method {
		case "mining.authorize":
			var p struct {
				Agent  string `json:"agent"`
				Type   string `json:"type"`
				Wallet string `json:"wallet"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			c.worker = p.Wallet
			log.Printf("authorized %s (%s)", p.Wallet, p.Agent)
			_ = c.send(map[string]any{
				"id": msg.ID, "result": true, "error": nil, "type": "v2",
			})
			if j := s.job(); j != nil {
				_ = c.send(s.notifyMsg(j))
			}

		case "mining.submit":
			s.handleSubmit(c, msg.ID, msg.Params, line)

		case "mining.keepalived", "mining.ping":
			_ = c.send(map[string]any{"id": msg.ID, "result": true, "error": nil})

		default:
			// Unknown methods are logged verbatim: this dialect is not
			// specified anywhere, so anything unexpected is information.
			log.Printf("unhandled method %q from %s: %s",
				msg.Method, conn.RemoteAddr(), truncate(string(line), 2000))
			_ = c.send(map[string]any{"id": msg.ID, "result": true, "error": nil})
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("… (%d bytes total)", len(s))
}

// meetsTarget reports whether a 32-byte big-endian value is at or under bits.
func meetsTarget(valueBE []byte, bits uint32) bool {
	v := new(big.Int).SetBytes(valueBE)
	return v.Cmp(blockchain.CompactToBig(bits)) <= 0
}

// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This file turns latstatus from a one-method shim into the read-only public
// API the website's wallet app runs on.
//
// The rule that shapes every line here: nothing on the public internet may
// reach anything that can move coins. The browser never sees node credentials,
// the endpoint table is a closed set, and every method behind it is read-only
// by construction. There is deliberately no path from this file to a wallet —
// signing happens in latwalletgui on the user's own machine, where a
// compromised web server cannot reach it.
//
// Inputs are validated against strict patterns before a request is built, so a
// crafted address or txid cannot smuggle anything into the JSON-RPC payload.

var (
	// Bech32 for Lattice's mainnet HRP. Deliberately narrow: this is the only
	// shape of address the API will look up.
	reAddress = regexp.MustCompile(`^lat1[02-9ac-hj-np-z]{20,90}$`)
	reHash    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reHeight  = regexp.MustCompile(`^[0-9]{1,9}$`)
)

// limiter is a small per-IP token bucket. The API is cheap to serve but sits
// in front of a node doing real work, so a single client should not be able to
// make it the bottleneck.
type limiter struct {
	mu     sync.Mutex
	seen   map[string]*bucket
	rate   float64 // tokens per second
	burst  float64
	lastGC time.Time
}

type bucket struct {
	tokens float64
	at     time.Time
}

func newLimiter(rate, burst float64) *limiter {
	return &limiter{seen: map[string]*bucket{}, rate: rate, burst: burst, lastGC: time.Now()}
}

func (l *limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.lastGC) > 10*time.Minute {
		for k, b := range l.seen {
			if now.Sub(b.at) > 10*time.Minute {
				delete(l.seen, k)
			}
		}
		l.lastGC = now
	}

	b, ok := l.seen[ip]
	if !ok {
		l.seen[ip] = &bucket{tokens: l.burst - 1, at: now}
		return true
	}
	b.tokens += now.Sub(b.at).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(r *http.Request) string {
	// Behind the Cloudflare tunnel the peer address is the tunnel, so prefer
	// the forwarded chain's first hop when present.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rpc calls one JSON-RPC method and returns its result.
func rpc(client *http.Client, method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "1.0", "id": "latstatus", "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", *nodeURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(*rpcUser, *rpcPass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("node returned unparseable JSON: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("%s", envelope.Error.Message)
	}
	return envelope.Result, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// chainSummary is the dashboard payload: cheap calls only. getblockchaininfo is
// avoided on purpose — it can block behind the CPU miner holding the chain
// lock, and a status widget must never hang the page.
func chainSummary(client *http.Client) (map[string]any, error) {
	height, err := rpc(client, "getblockcount")
	if err != nil {
		return nil, err
	}
	out := map[string]any{"height": json.RawMessage(height)}
	for key, method := range map[string]string{
		"difficulty": "getdifficulty",
		"bestblock":  "getbestblockhash",
		"peers":      "getconnectioncount",
	} {
		if v, err := rpc(client, method); err == nil {
			out[key] = json.RawMessage(v)
		}
	}
	if v, err := rpc(client, "getnextreset"); err == nil {
		out["emission"] = json.RawMessage(v)
	}
	return out, nil
}

// txOut and txIn mirror only the fields the app renders. Decoding into named
// structs rather than forwarding the node's raw reply keeps the public shape
// stable and drops anything the app has no business showing.
type verboseTx struct {
	Txid          string `json:"txid"`
	Confirmations int64  `json:"confirmations"`
	Time          int64  `json:"time"`
	Blockhash     string `json:"blockhash"`
	Vout          []struct {
		Value        float64 `json:"value"`
		N            uint32  `json:"n"`
		ScriptPubKey struct {
			Address   string   `json:"address"`
			Addresses []string `json:"addresses"`
		} `json:"scriptPubKey"`
	} `json:"vout"`
	Vin []struct {
		Txid     string `json:"txid"`
		Vout     uint32 `json:"vout"`
		Coinbase string `json:"coinbase"`
	} `json:"vin"`
}

func (t *verboseTx) paysTo(addr string) float64 {
	var sum float64
	for _, o := range t.Vout {
		if o.ScriptPubKey.Address == addr {
			sum += o.Value
			continue
		}
		for _, a := range o.ScriptPubKey.Addresses {
			if a == addr {
				sum += o.Value
				break
			}
		}
	}
	return sum
}

// addressSummary computes a balance from the address index.
//
// The node has no getaddressbalance, so the balance is derived: every output
// paying the address is credited, and any of those outputs later consumed by an
// input in the same result set is debited. searchrawtransactions returns both
// sides for the address, which is what makes the subtraction complete.
func addressSummary(client *http.Client, addr string, limit int) (map[string]any, error) {
	valid, err := rpc(client, "validateaddress", addr)
	if err != nil {
		return nil, err
	}
	var v struct {
		IsValid bool `json:"isvalid"`
	}
	if err := json.Unmarshal(valid, &v); err != nil || !v.IsValid {
		return nil, fmt.Errorf("not a valid Lattice address")
	}

	raw, err := rpc(client, "searchrawtransactions", addr, 1, 0, limit)
	if err != nil {
		// An address the index has never seen is not an error, it is empty.
		if strings.Contains(strings.ToLower(err.Error()), "no information") {
			return map[string]any{"address": addr, "balance": 0.0, "immature": 0.0,
				"txcount": 0, "transactions": []any{}}, nil
		}
		return nil, err
	}

	var txs []verboseTx
	if err := json.Unmarshal(raw, &txs); err != nil {
		return nil, fmt.Errorf("unexpected index reply: %w", err)
	}

	// Credit every output to the address, then debit the ones spent.
	type outpoint struct {
		txid string
		n    uint32
	}
	credited := map[outpoint]float64{}
	var immature float64
	for i := range txs {
		t := &txs[i]
		coinbase := len(t.Vin) > 0 && t.Vin[0].Coinbase != ""
		for _, o := range t.Vout {
			paid := o.ScriptPubKey.Address == addr
			if !paid {
				for _, a := range o.ScriptPubKey.Addresses {
					if a == addr {
						paid = true
						break
					}
				}
			}
			if !paid {
				continue
			}
			// Coinbase outputs are unspendable until they mature.
			if coinbase && t.Confirmations < coinbaseMaturity {
				immature += o.Value
				continue
			}
			credited[outpoint{t.Txid, o.N}] = o.Value
		}
	}
	for i := range txs {
		for _, in := range txs[i].Vin {
			delete(credited, outpoint{in.Txid, in.Vout})
		}
	}

	var balance float64
	for _, v := range credited {
		balance += v
	}

	list := make([]map[string]any, 0, len(txs))
	for i := range txs {
		t := &txs[i]
		list = append(list, map[string]any{
			"txid":          t.Txid,
			"time":          t.Time,
			"confirmations": t.Confirmations,
			"received":      t.paysTo(addr),
			"coinbase":      len(t.Vin) > 0 && t.Vin[0].Coinbase != "",
		})
	}

	return map[string]any{
		"address": addr, "balance": balance, "immature": immature,
		"txcount": len(txs), "transactions": list,
	}, nil
}

// coinbaseMaturity mirrors chaincfg's mainnet value. Duplicated rather than
// imported so this shim stays a small standalone binary.
const coinbaseMaturity = 100

// registerAPI wires the read-only endpoints. Each one validates its input, then
// calls a fixed method; there is no pass-through of a caller-supplied method.
//
// Everything lives under /api/ so site ingress needs exactly one extra route
// beyond the original /status, which stays where it was so the existing page
// keeps working.
func registerAPI(mux *http.ServeMux, client *http.Client) {
	lim := newLimiter(5, 20) // 5 req/s sustained, burst 20, per IP
	caches := map[string]*cached{"chain": {}, "address": {}, "tx": {}, "block": {}}
	var cacheMu sync.Mutex

	cacheFor := func(key string) *cached {
		cacheMu.Lock()
		defer cacheMu.Unlock()
		c, ok := caches[key]
		if !ok {
			c = &cached{}
			caches[key] = c
		}
		return c
	}

	guard := func(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", *origin)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			if !lim.allow(clientIP(r)) {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "slow down"})
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/api/chain", guard(func(w http.ResponseWriter, r *http.Request) {
		body, err := cacheFor("chain").get(func() ([]byte, error) {
			s, err := chainSummary(client)
			if err != nil {
				return nil, err
			}
			return json.Marshal(s)
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "node unavailable"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))

	mux.HandleFunc("/api/address", guard(func(w http.ResponseWriter, r *http.Request) {
		addr := r.URL.Query().Get("a")
		if !reAddress.MatchString(addr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed address"})
			return
		}
		body, err := cacheFor("address:" + addr).get(func() ([]byte, error) {
			s, err := addressSummary(client, addr, 200)
			if err != nil {
				return nil, err
			}
			return json.Marshal(s)
		})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))

	mux.HandleFunc("/api/tx", guard(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if !reHash.MatchString(id) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed txid"})
			return
		}
		body, err := cacheFor("tx:" + id).get(func() ([]byte, error) {
			res, err := rpc(client, "getrawtransaction", id, 1)
			return res, err
		})
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "transaction not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))

	mux.HandleFunc("/api/block", guard(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		hash := id
		switch {
		case reHash.MatchString(id):
			// already a hash
		case reHeight.MatchString(id):
			h, err := strconv.Atoi(id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed height"})
				return
			}
			res, err := rpc(client, "getblockhash", h)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "no block at that height"})
				return
			}
			if err := json.Unmarshal(res, &hash); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "node unavailable"})
				return
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "give a block hash or height"})
			return
		}
		body, err := cacheFor("block:" + hash).get(func() ([]byte, error) {
			return rpc(client, "getblock", hash, 1)
		})
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "block not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
}

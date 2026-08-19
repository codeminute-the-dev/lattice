// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command latwalletgui serves a local web UI for a running latwallet.
//
// It is a thin, deliberately boring bridge: it serves one HTML page and
// forwards a fixed whitelist of JSON-RPC methods to latwallet and latticed.
// It holds no keys, stores nothing on disk, and keeps no state beyond the
// per-run auth token.
//
// # Why the token
//
// The wallet RPC listens on loopback, which stops other machines but not the
// browser already running on this one: any page you visit can POST to
// 127.0.0.1. So every /api call must carry the token printed at startup in an
// X-Lat-Token header. A cross-origin page cannot read that token, and a custom
// header forces a CORS preflight it cannot satisfy. The Origin check below is
// the second lock on the same door.
//
// # What it will not do
//
// dumpprivkey, dumpwallet, and importprivkey are absent from the whitelist on
// purpose. A browser UI is the wrong place to move raw key material, and a
// bridge that cannot express those calls cannot be tricked into making them.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codeminute-the-dev/lattice/wallet/internal/walletui"
)

var (
	listen     = flag.String("listen", "127.0.0.1:8090", "address to serve the UI on")
	walletRPC  = flag.String("walletrpc", "127.0.0.1:44207", "latwallet RPC address")
	nodeRPC    = flag.String("noderpc", "127.0.0.1:44107", "latticed RPC address")
	rpcUser    = flag.String("rpcuser", "", "RPC username (defaults to $LATTICE_RPCUSER)")
	rpcPass    = flag.String("rpcpass", "", "RPC password (defaults to $LATTICE_RPCPASS)")
	configFile = flag.String("conf", "", "read rpcuser/rpcpass from this latticed.conf")
)

// walletMethods and nodeMethods are the complete set of calls the UI can make.
// Anything not listed here is refused before a request is built.
var walletMethods = map[string]bool{
	"getbalance":            true,
	"getnewaddress":         true,
	"getaccountaddress":     true,
	"listtransactions":      true,
	"listunspent":           true,
	"validateaddress":       true,
	"sendtoaddress":         true,
	"walletpassphrase":      true,
	"walletlock":            true,
	"settxfee":              true,
	"signmessage":           true,
	"gettransaction":        true,
	"listreceivedbyaddress": true,
}

var nodeMethods = map[string]bool{
	"getblockcount":      true,
	"getblockchaininfo":  true,
	"getnextreset":       true,
	"getconnectioncount": true,
	"getmininginfo":      true,
	"getdifficulty":      true,
}

// sensitive marks methods whose params must never reach a log line.
var sensitive = map[string]bool{
	"walletpassphrase": true,
	"sendtoaddress":    true,
	"signmessage":      true,
}

var token string

type rpcReq struct {
	Target string            `json:"target"` // "wallet" or "node"
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func loadCreds() error {
	if *rpcUser == "" {
		*rpcUser = os.Getenv("LATTICE_RPCUSER")
	}
	if *rpcPass == "" {
		*rpcPass = os.Getenv("LATTICE_RPCPASS")
	}
	if *configFile != "" {
		f, err := os.ReadFile(*configFile)
		if err != nil {
			return fmt.Errorf("reading %s: %w", *configFile, err)
		}
		for _, line := range strings.Split(string(f), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "rpcuser="):
				*rpcUser = strings.TrimPrefix(line, "rpcuser=")
			case strings.HasPrefix(line, "rpcpass="):
				*rpcPass = strings.TrimPrefix(line, "rpcpass=")
			}
		}
	}
	if *rpcUser == "" || *rpcPass == "" {
		return fmt.Errorf("no RPC credentials: pass -conf, -rpcuser/-rpcpass, or set LATTICE_RPCUSER/LATTICE_RPCPASS")
	}
	return nil
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// call forwards one JSON-RPC request upstream and returns the raw response body.
func call(addr, method string, params []json.RawMessage) ([]byte, error) {
	if params == nil {
		params = []json.RawMessage{}
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "1.0", "id": "latwalletgui", "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "http://"+addr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(*rpcUser, *rpcPass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// localOrigin reports whether an Origin header, if present, is one of ours.
// A missing Origin is fine: that is a same-origin fetch or a direct load.
func localOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !localOrigin(r) {
		http.Error(w, "bad origin", http.StatusForbidden)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Lat-Token")), []byte(token)) != 1 {
		http.Error(w, "bad token", http.StatusForbidden)
		return
	}

	var req rpcReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var addr string
	switch req.Target {
	case "wallet":
		if !walletMethods[req.Method] {
			http.Error(w, "method not allowed", http.StatusForbidden)
			return
		}
		addr = *walletRPC
	case "node":
		if !nodeMethods[req.Method] {
			http.Error(w, "method not allowed", http.StatusForbidden)
			return
		}
		addr = *nodeRPC
	default:
		http.Error(w, "unknown target", http.StatusBadRequest)
		return
	}

	// Params are omitted for anything that can carry a passphrase or an amount.
	if sensitive[req.Method] {
		log.Printf("rpc %s %s (params withheld)", req.Target, req.Method)
	} else {
		log.Printf("rpc %s %s", req.Target, req.Method)
	}

	out, err := call(addr, req.Method, req.Params)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"result": nil,
			"error":  map[string]any{"message": err.Error()},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func main() {
	flag.Parse()
	if err := loadCreds(); err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("generating token: %v", err)
	}
	token = hex.EncodeToString(buf)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/rpc", apiHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		// The page needs its own inline style and script and same-origin XHR.
		h.Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
				"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Type", "text/html; charset=utf-8")
		// The token is templated into the page so a same-origin load is
		// authenticated while a cross-origin one cannot read it.
		w.Write([]byte(strings.Replace(walletui.IndexHTML, "__TOKEN__", token, 1)))
	})

	uiURL := fmt.Sprintf("http://%s/?t=%s", *listen, token)
	log.Printf("Lattice wallet UI: %s", uiURL)
	log.Printf("(the token in that URL authenticates the page; keep it to yourself)")

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

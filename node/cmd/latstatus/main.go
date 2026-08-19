// Command latstatus exposes a latticed node's public chain data as a small
// read-only JSON API, so a static site can render a live countdown, a chain
// dashboard, and the website's watch-only wallet app.
//
// It exists because a browser cannot call latticed's JSON-RPC directly: that
// interface uses HTTP basic auth over TLS and sends no CORS headers, and putting
// node RPC credentials in a web page would be a poor idea regardless. This shim
// holds the credentials server-side and re-publishes a closed set of read-only
// methods, with validated parameters and no way to reach anything else.
//
// Every method behind it is read-only by construction. There is deliberately no
// endpoint that can move coins and no path from here to a wallet: signing lives
// in latwalletgui on the user's own machine, so compromising this server cannot
// cost anyone their funds.
//
// Usage:
//
//	latstatus -node http://127.0.0.1:44107 -user rpcuser -pass rpcpass -listen :8080
//
// Then point the website's API_BASE at http://your-host:8080.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	nodeURL  = flag.String("node", "http://127.0.0.1:44107", "latticed JSON-RPC URL")
	confPath = flag.String("conf", "", "read rpcuser/rpcpass from a latticed.conf (preferred)")
	rpcUser  = flag.String("user", "", "node RPC username (avoid: visible in ps)")
	rpcPass  = flag.String("pass", "", "node RPC password (avoid: visible in ps)")
	listen   = flag.String("listen", ":8080", "address to serve on")
	origin   = flag.String("origin", "*", "value for Access-Control-Allow-Origin")
	cacheTTL = flag.Duration("cache", 5*time.Second, "how long to reuse a node response")
)

type cached struct {
	mu   sync.Mutex
	body []byte
	at   time.Time
}

func (c *cached) get(fetch func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.body != nil && time.Since(c.at) < *cacheTTL {
		return c.body, nil
	}
	b, err := fetch()
	if err != nil {
		// Serve the last good response rather than nothing if the node
		// blips; the schedule barely moves between polls anyway.
		if c.body != nil {
			return c.body, nil
		}
		return nil, err
	}
	c.body, c.at = b, time.Now()
	return b, nil
}

func fetchNextReset(client *http.Client) ([]byte, error) {
	payload := `{"jsonrpc":"1.0","id":"latstatus","method":"getnextreset","params":[]}`
	req, err := http.NewRequest("POST", *nodeURL, bytes.NewBufferString(payload))
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

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("node returned unparseable JSON: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("node error: %s", envelope.Error.Message)
	}
	return envelope.Result, nil
}

// loadConf reads rpcuser and rpcpass out of a latticed.conf. Passing
// credentials on the command line would expose them to every user on the box
// via ps, so this is the preferred path.
func loadConf(path string) (user, pass string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "rpcuser":
			user = strings.TrimSpace(v)
		case "rpcpass":
			pass = strings.TrimSpace(v)
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if user == "" || pass == "" {
		return "", "", fmt.Errorf("%s contains no rpcuser/rpcpass", path)
	}
	return user, pass, nil
}

func main() {
	flag.Parse()

	if *confPath != "" {
		u, p, err := loadConf(*confPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "latstatus: %v\n", err)
			os.Exit(2)
		}
		*rpcUser, *rpcPass = u, p
	}
	if *rpcUser == "" || *rpcPass == "" {
		fmt.Fprintln(os.Stderr, "latstatus: need -conf (preferred) or -user/-pass")
		os.Exit(2)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	c := &cached{}

	mux := http.NewServeMux()
	registerAPI(mux, client)

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", *origin)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		body, err := c.get(func() ([]byte, error) { return fetchNextReset(client) })
		if err != nil {
			log.Printf("upstream: %v", err)
			http.Error(w, `{"error":"node unavailable"}`, http.StatusBadGateway)
			return
		}
		w.Write(body)
	})

	log.Printf("latstatus serving %s from node %s", *listen, *nodeURL)
	log.Printf("endpoints: /status /api/chain /api/address?a= /api/tx?id= /api/block?id=")
	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

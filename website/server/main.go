// Command latsite serves the static Lattice website.
//
// Deliberately minimal: it serves one directory read-only, sets a few sensible
// headers, and does nothing else. It sits behind a Cloudflare tunnel, which
// terminates TLS, so there is none of that here.
package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

var (
	dir    = flag.String("dir", "website", "directory to serve")
	listen = flag.String("listen", "127.0.0.1:8081", "address to listen on")
)

// secureHeaders applies a conservative header set. The CSP mirrors what the
// page actually needs: its own inline styles and script, Google Fonts, and
// same-origin XHR for /status. Nothing else is permitted.
func secureHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"script-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")

		// The page is small and changes rarely; a short cache keeps it
		// fresh without hammering the origin through the tunnel.
		if strings.HasSuffix(r.URL.Path, ".html") || r.URL.Path == "/" {
			h.Set("Cache-Control", "public, max-age=300")
		} else {
			h.Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	flag.Parse()
	root, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(root)))

	srv := &http.Server{
		Addr:              *listen,
		Handler:           secureHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	log.Printf("latsite serving %s on %s", root, *listen)
	log.Fatal(srv.ListenAndServe())
}

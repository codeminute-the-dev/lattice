package jsonrpccache

import (
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("jsonrpc_cache", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("jsonrpc_cache", "before", "reverse_proxy")
}

// parseCaddyfile unmarshals tokens from h into a new Handler.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var handler Handler
	err := handler.UnmarshalCaddyfile(h.Dispenser)
	return &handler, err
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler. Syntax:
//
//	jsonrpc_cache {
//	    cache <method> <ttl>
//	}
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			return d.ArgErr()
		}

		for nesting := d.Nesting(); d.NextBlock(nesting); {
			switch d.Val() {
			case "cache":
				args := d.RemainingArgs()
				if len(args) != 2 {
					return d.ArgErr()
				}
				ttl, err := time.ParseDuration(args[1])
				if err != nil {
					return d.Errf("invalid TTL %q: %v", args[1], err)
				}
				h.Rules = append(h.Rules, CacheRule{
					Method: args[0],
					TTL:    caddy.Duration(ttl),
				})
			default:
				return d.Errf("unrecognized subdirective '%s'", d.Val())
			}
		}
	}
	return nil
}

// Interface guard
var _ caddyfile.Unmarshaler = (*Handler)(nil)

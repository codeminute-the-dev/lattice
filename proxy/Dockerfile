FROM caddy:2-builder AS builder

COPY caddy-jsonrpc-cache/ /src/caddy-jsonrpc-cache/

RUN xcaddy build \
    --with github.com/mholt/caddy-ratelimit \
    --with github.com/codeminute-the-dev/lattice/proxy/caddy-jsonrpc-cache=/src/caddy-jsonrpc-cache

FROM caddy:2

COPY --from=builder /usr/bin/caddy /usr/bin/caddy
COPY Caddyfile /etc/caddy/Caddyfile

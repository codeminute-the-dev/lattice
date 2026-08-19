# Lattice RPC Proxy

Caddy-based reverse proxy sidecar for Lattice node RPC. Provides TLS termination,
per-IP rate limiting, and optional `getblocktemplate` response caching.

The proxy runs as a sidecar container sharing the node's network namespace.
`latticed` binds to `127.0.0.1:18334` (plaintext, `--notls`), and Caddy handles
all external-facing TLS on port `44107`.

## Quick Start (Docker)

```bash
# Build the images
docker build -t latticed -f node/Dockerfile .
docker build -t latticed-proxy proxy/

# Start latticed
docker run -d --name latticed \
  -p 44108:44108 \
  -p 44107:44107 \
  latticed \
  --notls \
  --rpclisten=127.0.0.1:18334 \
  --rpcuser=admin \
  --rpcpass=changeme

# Start the Caddy sidecar (shares latticed's network namespace)
docker run -d --name proxy \
  --network=container:latticed \
  latticed-proxy
```

RPC is now available at `https://localhost:44107` with a self-signed certificate.

## Docker Compose

```yaml
services:
  node:
    build:
      context: .
      dockerfile: node/Dockerfile
    command:
      - "--notls"
      - "--rpclisten=127.0.0.1:18334"
      - "--rpcuser=${RPC_USER:-admin}"
      - "--rpcpass=${RPC_PASS:-changeme}"
    volumes:
      - latticed-data:/root/.latticed
    ports:
      - "44108:44108"   # P2P
      - "44107:44107"   # RPC (served by Caddy via shared namespace)

  proxy:
    build:
      context: proxy/
    network_mode: "service:node"
    volumes:
      - ./proxy/Caddyfile:/etc/caddy/Caddyfile:ro
    depends_on:
      - node

volumes:
  latticed-data:
```

Port `44107` is declared on the `node` service because `network_mode: "service:node"`
shares its network namespace — both containers see the same `localhost`.

## TLS Modes

### Self-signed (default)

The default `Caddyfile` uses `tls internal`. Caddy generates a local CA root
and issues a leaf certificate automatically. The root CA is stored at:

```
/data/caddy/pki/authorities/local/root.crt
```

To extract it from the running container:

```bash
docker cp proxy:/data/caddy/pki/authorities/local/root.crt lattice-ca.crt
```

Then add `lattice-ca.crt` to your client's trust store, or pass it explicitly:

```bash
curl --cacert lattice-ca.crt https://localhost:44107 \
  -u admin:changeme \
  -d '{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}'
```

### Domain with automatic ACME (Let's Encrypt)

See `Caddyfile.domain.example`. Replace the domain and Caddy handles certificate
issuance and renewal automatically.

### Bring your own certificate

```
:44107 {
    tls /path/to/cert.pem /path/to/key.pem
    ...
}
```

Mount the cert/key files into the proxy container and reference them in the
Caddyfile.

## Rate Limiting

The default config limits each remote IP to **100 requests per minute**. Excess
requests receive HTTP 429 (Too Many Requests). Adjust in the `Caddyfile`:

```
rate_limit {
    zone rpc {
        key    {remote_host}
        events 120
        window 1m
    }
}
```

## Kubernetes Deployment

In Kubernetes, containers in the same Pod share a network namespace by default.
This is the recommended deployment pattern:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: latticed
spec:
  containers:
    - name: latticed
      image: ghcr.io/codeminute-the-dev/latticed:latest
      args:
        - "--notls"
        - "--rpclisten=127.0.0.1:18334"
        - "--rpcuser=$(RPC_USER)"
        - "--rpcpass=$(RPC_PASS)"
      env:
        - name: RPC_USER
          valueFrom:
            secretKeyRef:
              name: latticed-rpc
              key: username
        - name: RPC_PASS
          valueFrom:
            secretKeyRef:
              name: latticed-rpc
              key: password
      ports:
        - containerPort: 44108
          name: p2p
      volumeMounts:
        - name: data
          mountPath: /root/.latticed

    - name: proxy
      image: ghcr.io/codeminute-the-dev/latticed-proxy:latest
      ports:
        - containerPort: 44107
          name: rpc-tls
      volumeMounts:
        - name: caddy-config
          mountPath: /etc/caddy/Caddyfile
          subPath: Caddyfile

  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: latticed-data
    - name: caddy-config
      configMap:
        name: latticed-caddyfile
```

Expose via a `Service`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: latticed
spec:
  selector:
    app: latticed
  ports:
    - name: rpc-tls
      port: 44107
      targetPort: rpc-tls
    - name: p2p
      port: 44108
      targetPort: p2p
```

## Testing

The smoke test script validates TLS, auth, rate limiting, and caching against
any running Lattice node:

```bash
./proxy/proxy_test.sh \
    --node-rpc=node1.internal.lattice.codeminute.dev:44107 \
    --rpc-user=admin \
    --rpc-pass=pass
```

The script builds the proxy image, starts a Caddy container pointing at the
given node, runs 10 tests, and tears down. It works with any reachable node
(testnet, mainnet, or local).

The `caddy-jsonrpc-cache` plugin also has Go unit tests:

```bash
cd proxy/caddy-jsonrpc-cache && go test ./...
```

## Building with Plugins

The proxy Dockerfile builds a custom Caddy binary via `xcaddy` with two plugins:

- [`caddy-ratelimit`](https://github.com/mholt/caddy-ratelimit) — per-IP rate limiting
- `caddy-jsonrpc-cache` (local module in `caddy-jsonrpc-cache/`) — JSON-RPC method-aware response caching

To add additional Caddy plugins, append `--with` flags in the Dockerfile's
`xcaddy build` command.

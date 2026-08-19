# Running Lattice as a service

What is currently live at `lattice.codeminute.dev` runs as four processes:

| Process | Port | Role |
|---|---|---|
| `latticed` | 44107 (RPC), 44108 (P2P) | The mainnet full node |
| `latstatus` | 8099 | Re-publishes `getnextreset` read-only, for the website |
| `latsite` | 8081 | Serves `website/` as static files |
| `cloudflared` | — | Tunnel for `lattice.codeminute.dev` |

The units in `deploy/systemd/` make that survive a reboot. Install them with:

```bash
cp deploy/systemd/*.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now latticed latstatus latsite lattice-tunnel

# so they start at boot rather than at login
sudo loginctl enable-linger "$USER"
```

Check on them:

```bash
systemctl --user status latticed latstatus latsite lattice-tunnel
journalctl --user -u latticed -f
```

## Credentials

`latticed` reads `~/.latticed/latticed.conf`, which holds the RPC username and
password and should stay mode `600`. `latstatus` reads the same file via
`-conf`, so the credentials never appear in `ps` output or in a unit file.

RPC is bound to `127.0.0.1` with TLS disabled. That is deliberate and safe here:
nothing off-machine can reach the port, and the only thing exposed publicly is
`latstatus`, which accepts no parameters and can call exactly one read-only
method.

## The tunnel

`lattice.codeminute.dev` uses its own Cloudflare tunnel (`lattice`,
`92379099-e767-4549-ba72-ac4f0bf9c195`) with config at
`~/.cloudflared/lattice.yml`. It is deliberately separate from the `wormt`
tunnel in `/etc/cloudflared`, so restarting one never interrupts the other.

Ingress routes `/status` to `latstatus` and everything else to `latsite`, both
on loopback.

## Updating the site

`latsite` serves from disk, so editing `website/index.html` takes effect on the
next request. No restart, no rebuild.

## Mining

The node runs without a mining address. Mainnet has `GenerateSupported: false`,
so it will not CPU-mine, and the chain stays at height 0 until a GPU miner is
pointed at it. To mine, add a `miningaddr` to `latticed.conf` and run the
gateway — see [`../docs/solo-mining.md`](../docs/solo-mining.md).

For CPU mining while developing, use simnet or regtest, which do support
`generate`.

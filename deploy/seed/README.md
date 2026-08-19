# Running a Lattice seed node

A seed node is how anyone else finds the network. It does not mine and holds no
wallet — it listens on port 44108, accepts inbound connections, and gossips peer
addresses. That is the whole job.

It exists because the machine running the chain usually cannot advertise it.
Most home connections cannot accept inbound connections at all; behind CGNAT
there is no port to forward, because the public address belongs to the carrier
and is shared with thousands of other subscribers. A node in that position still
participates fully — it dials out, syncs, relays, and mines — it just cannot be
the thing newcomers connect *to*.

So exactly one machine needs a real address. Everything else, including the
mining node at home, dials out to it.

## Picking a box

The build is the constraint, not the running node. `latticed` idles at around
150 MB, but the verifier circuit caches are not committed to git, so a fresh
install generates them — a plonky2 recursion build that wants several GB.

| Shape | Verdict |
| --- | --- |
| Oracle Ampere A1 (ARM, up to 4 OCPU / 24 GB, always free) | **Use this.** Comfortable for the build |
| Oracle VM.Standard.E2.1.Micro (x86, 1 OCPU / 1 GB, always free) | Runs the node fine, but will be OOM-killed generating the caches. Build elsewhere and copy `bin/latticed`, `zk-pow/src/circuit/v2_cache.bin` and `zk-pow/src/v1/v1_cache.bin` |
| Hetzner CX22 (~€4/mo, 2 vCPU / 4 GB) | Fine, and no capacity roulette |

Ampere capacity in popular Oracle regions is frequently exhausted. If the
console refuses, try a different availability domain or region.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/codeminute-the-dev/lattice/main/deploy/seed/bootstrap.sh -o bootstrap.sh
less bootstrap.sh          # it runs as root; read it first
sudo bash bootstrap.sh
```

It installs Go and Rust, builds `latticed`, creates an unprivileged `lattice`
user, writes a seed config with freshly generated RPC credentials, installs a
hardened systemd unit, and opens 44108 in the local firewall. Re-running it
upgrades in place.

## The two steps the script cannot do

**1. Oracle's cloud firewall.** Oracle filters in two places, and the script can
only reach one of them. In the console: *Networking > Virtual Cloud Networks >
your VCN > Security Lists* (or the instance's Network Security Group) — add an
ingress rule, source `0.0.0.0/0`, protocol TCP, destination port `44108`.

Until that rule exists the port is open on the machine and still unreachable
from the internet, which looks exactly like the node being broken.

**2. DNS.** Point `seeder1.lattice.codeminute.dev` at the instance's public IP
with an A record. That is the entire seeder: `connmgr.SeedFromDNS` does a plain
hostname lookup and dials every address it gets back on the network's default
port, so no seeder daemon is needed to start. The `dnsseeder` in this repo is
for later, when there are enough nodes that a static record stops being honest.

## Checking it worked

On the seed:

```bash
systemctl status latticed-seed
journalctl -u latticed-seed -f
```

From anywhere else — this is the test that matters, because it goes through both
firewalls:

```bash
nc -vz <public-ip> 44108
```

Then point your home node at it by adding to `~/.latticed/latticed.conf`:

```
addpeer=<public-ip>:44108
```

and restarting. `latctl getconnectioncount` should climb, and
`latctl getpeerinfo` should show the seed's address rather than a loopback one.

Once a real peer exists, the local `latticed-peer` stand-in is no longer needed:

```bash
systemctl --user disable --now latticed-peer
```

## Keeping it honest

The seed validates every block it relays — it runs the same consensus code as
any other node, with no mining and no keys. If it ever falls out of sync, it
stops being useful as a discovery point, so it is worth watching:

```bash
watch -n 60 'systemctl is-active latticed-seed'
```

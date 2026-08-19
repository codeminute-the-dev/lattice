#!/usr/bin/env bash
# Bootstrap a Lattice seed node on a fresh Ubuntu box.
#
# A seed node exists so other people can find the network. It does not mine and
# holds no wallet: it listens on 44108, accepts inbound connections, and gossips
# addresses. That is the whole job.
#
# It is needed because most home connections (CGNAT especially) cannot accept
# inbound connections, so the machine that runs the chain usually cannot be the
# machine that advertises it.
#
#   curl -fsSL <this file> -o bootstrap.sh && sudo bash bootstrap.sh
#
# Idempotent: safe to re-run to upgrade.
set -euo pipefail

REPO="${REPO:-https://github.com/codeminute-the-dev/lattice}"
BRANCH="${BRANCH:-main}"
SRC="${SRC:-/opt/lattice}"
GO_VERSION="${GO_VERSION:-1.26.6}"
RUN_USER="${RUN_USER:-lattice}"

say() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
die() { printf '\n\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run with sudo"

# ---------------------------------------------------------------- sanity
ARCH="$(uname -m)"
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

RAM_MB=$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)
say "Architecture $ARCH (Go: $GOARCH), RAM ${RAM_MB} MB"

# The verifier circuit caches are not in git, so a fresh clone has to generate
# them. Measured on a fast multi-core box: 1:40 wall clock and 1.18 GB peak
# RSS. The build is highly parallel, so a slower box costs mostly time rather
# than memory — but 1.18 GB does not fit in a 1 GB instance without swap.
if [ "$RAM_MB" -lt 1600 ]; then
  cat >&2 <<WARN

  This box has ${RAM_MB} MB of RAM. Generating the verifier circuit caches
  peaks around 1.2 GB, so it will likely be OOM-killed here.

  Three ways through, cheapest first:
    - Add swap, which is enough on a 1 GB instance:
        fallocate -l 2G /swapfile && chmod 600 /swapfile
        mkswap /swapfile && swapon /swapfile
        echo '/swapfile none swap sw 0 0' >> /etc/fstab
    - Use an Ampere (ARM) shape: Oracle's always-free tier gives 4 OCPU
      and 24 GB, with room to spare.
    - Build the caches elsewhere and copy them over. They are reproducible —
      the same source produces byte-identical files — so copying is exactly
      as trustworthy as building, provided you check the hashes:
        zk-pow/src/circuit/v2_cache.bin
        zk-pow/src/v1/v1_cache.bin

  Continuing anyway in 15s; Ctrl-C to stop.

WARN
  sleep 15
fi

# ---------------------------------------------------------------- packages
say "Installing build dependencies"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq build-essential pkg-config git curl ca-certificates

# ---------------------------------------------------------------- toolchains
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GO_VERSION}"; then
  say "Installing Go ${GO_VERSION} (${GOARCH})"
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tgz
  rm -f /tmp/go.tgz
fi
export PATH=/usr/local/go/bin:$PATH

if ! command -v cargo >/dev/null 2>&1; then
  say "Installing Rust"
  curl -fsSL https://sh.rustup.rs | sh -s -- -y --profile minimal --default-toolchain stable
fi
export PATH="$HOME/.cargo/bin:$PATH"

# ---------------------------------------------------------------- user
if ! id "$RUN_USER" >/dev/null 2>&1; then
  say "Creating service user $RUN_USER"
  useradd --system --create-home --home-dir "/var/lib/$RUN_USER" --shell /usr/sbin/nologin "$RUN_USER"
fi

# ---------------------------------------------------------------- source
if [ -d "$SRC/.git" ]; then
  say "Updating source in $SRC"
  git -C "$SRC" fetch --depth 1 origin "$BRANCH"
  git -C "$SRC" reset --hard "origin/$BRANCH"
else
  say "Cloning $REPO into $SRC"
  git clone --depth 1 --branch "$BRANCH" "$REPO" "$SRC"
fi
# No submodules are initialised on purpose. The only one is CUTLASS under
# miner/lattice-gemm, which is 175 MB of CUDA headers for a GPU miner. A seed
# node neither mines nor has a GPU.

# ---------------------------------------------------------------- build
cd "$SRC"
say "Building the verifier circuit caches (~2 min on 4+ cores, ~1.2 GB peak)"
if [ ! -s zk-pow/src/circuit/v2_cache.bin ] || [ ! -s zk-pow/src/v1/v1_cache.bin ]; then
  ( cd zk-pow && cargo run --release --no-default-features --bin build_cache \
      src/circuit/v2_cache.bin src/v1/v1_cache.bin )
else
  echo "  caches already present, skipping"
fi

say "Building the Rust FFI library"
( cd zk-pow/bindings/go && cargo build --release )

say "Building libxmss"
( cd xmss && make )

say "Building latticed"
export CGO_ENABLED=1 CGO_LDFLAGS_ALLOW=".*zk_pow_ffi.*" GOFLAGS=-mod=mod
mkdir -p bin
go build -tags xmss,zkpow -o bin/latticed ./node
go build -tags xmss,zkpow -o bin/latctl ./node/cmd/latctl
./bin/latticed --version || true

# ---------------------------------------------------------------- config
CONF_DIR="/var/lib/$RUN_USER/.latticed"
install -d -o "$RUN_USER" -g "$RUN_USER" -m 700 "$CONF_DIR"

if [ ! -f "$CONF_DIR/latticed.conf" ]; then
  say "Writing seed node config"
  RPCUSER="lattice_$(head -c 9 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 8)"
  RPCPASS="$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)"
  # externalip makes the node advertise the address peers should dial, rather
  # than whatever it guesses from its own interface (which on a cloud box is a
  # private address behind the provider's NAT).
  PUBIP="$(curl -fsS --max-time 10 -4 https://api.ipify.org || true)"
  cat > "$CONF_DIR/latticed.conf" <<CONF
; Lattice seed node.
;
; No mining, no wallet, no indexes: this node exists to be reachable and to
; hand out peer addresses. Keep it boring.

rpcuser=$RPCUSER
rpcpass=$RPCPASS

; RPC stays on loopback. Nothing outside this box needs it, and TLS is pointless
; for a loopback socket.
rpclisten=127.0.0.1:44107
notls=1

; The public P2P port. This is the one thing that must be reachable.
listen=0.0.0.0:44108
${PUBIP:+externalip=$PUBIP:44108}
CONF
  chown "$RUN_USER:$RUN_USER" "$CONF_DIR/latticed.conf"
  chmod 600 "$CONF_DIR/latticed.conf"
fi

# ---------------------------------------------------------------- service
say "Installing systemd unit"
cat > /etc/systemd/system/latticed-seed.service <<UNIT
[Unit]
Description=Lattice seed node
Documentation=https://github.com/codeminute-the-dev/lattice
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$RUN_USER
Group=$RUN_USER
ExecStart=$SRC/bin/latticed --configfile=$CONF_DIR/latticed.conf --datadir=/var/lib/$RUN_USER/data --logdir=/var/lib/$RUN_USER/logs
# A plain SIGTERM counts as a clean stop, so on-failure would not bring the seed
# back after a stray kill. The whole network's discovery depends on this staying up.
Restart=always
RestartSec=10

NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/$RUN_USER
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictSUIDSGID=true

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now latticed-seed

# ---------------------------------------------------------------- firewall
say "Opening port 44108 locally"
# Oracle's Ubuntu images ship with a restrictive iptables INPUT chain that drops
# everything not explicitly allowed. This only fixes the box; the Security List
# or NSG in the Oracle console has to allow 44108 as well, and that part cannot
# be done from here.
if command -v iptables >/dev/null 2>&1; then
  iptables -C INPUT -p tcp --dport 44108 -j ACCEPT 2>/dev/null || \
    iptables -I INPUT 1 -p tcp --dport 44108 -j ACCEPT
  command -v netfilter-persistent >/dev/null 2>&1 && netfilter-persistent save || \
    { command -v iptables-save >/dev/null 2>&1 && mkdir -p /etc/iptables && iptables-save > /etc/iptables/rules.v4; }
fi
command -v ufw >/dev/null 2>&1 && ufw allow 44108/tcp || true

# ---------------------------------------------------------------- done
sleep 5
PUBIP="$(curl -fsS --max-time 10 -4 https://api.ipify.org || echo '<your-ip>')"
cat <<DONE

  Seed node is up.

    status   systemctl status latticed-seed
    logs     journalctl -u latticed-seed -f
    height   $SRC/bin/latctl -u \$(sed -n 's/^rpcuser=//p' $CONF_DIR/latticed.conf) \\
               -P \$(sed -n 's/^rpcpass=//p' $CONF_DIR/latticed.conf) \\
               -s 127.0.0.1:44107 --notls getblockcount

  Two things left, and neither can be done from this box:

    1. Oracle console: VCN > Security Lists (or the instance's NSG) >
       add an ingress rule, source 0.0.0.0/0, TCP, destination port 44108.
       Without this the port is open on the machine and still unreachable.

    2. DNS: point seeder1.lattice.codeminute.dev at
         $PUBIP
       An A record is the entire seeder — latticed resolves the hostname and
       dials every address it gets back on port 44108.

  Then on your home node, add:  addpeer=$PUBIP:44108

DONE

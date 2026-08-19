#!/usr/bin/env bash
# Expose a Lattice node as a Tor hidden service.
#
# This is the answer to "my connection cannot accept inbound connections and I
# do not want to rent a server". A hidden service reaches the network by dialling
# out to introduction points, so CGNAT does not matter, there is no port to
# forward, no account to open, and no card to hand over.
#
# What it costs: peers need Tor themselves to reach an onion address, and DNS
# seeds cannot advertise one — an A record holds an IP, not a 56-character
# onion name. So an onion seed is published by hand rather than discovered, and
# it complements a clearnet seed rather than replacing one.
#
#   sudo bash tor-seed.sh
#
# Idempotent: re-running prints the existing address rather than making another.
set -euo pipefail

TORRC="${TORRC:-/etc/tor/torrc}"
HS_DIR="${HS_DIR:-/var/lib/tor/lattice-seed}"
P2P_PORT="${P2P_PORT:-44108}"
LOCAL_ADDR="${LOCAL_ADDR:-127.0.0.1}"

[ "$(id -u)" -eq 0 ] || { echo "run with sudo" >&2; exit 1; }
command -v tor >/dev/null 2>&1 || { echo "tor is not installed: apt install tor" >&2; exit 1; }

if grep -q "^HiddenServiceDir $HS_DIR" "$TORRC"; then
  echo "Hidden service already configured in $TORRC — leaving it alone."
else
  echo "Appending a hidden service to $TORRC (existing entries untouched)"
  cp -a "$TORRC" "$TORRC.bak-$(date +%s)"
  cat >> "$TORRC" <<CONF

# Lattice seed node. Added by deploy/seed/tor-seed.sh.
# Only the P2P port is exposed; the RPC port is deliberately not listed here.
HiddenServiceDir $HS_DIR
HiddenServicePort $P2P_PORT $LOCAL_ADDR:$P2P_PORT
CONF
  systemctl restart tor
fi

# The hostname file appears a moment after tor reloads.
for _ in $(seq 1 30); do
  [ -s "$HS_DIR/hostname" ] && break
  sleep 1
done
[ -s "$HS_DIR/hostname" ] || { echo "tor did not produce $HS_DIR/hostname; check: journalctl -u tor -n 40" >&2; exit 1; }

ONION="$(cat "$HS_DIR/hostname")"

cat <<DONE

  Onion seed address:

    $ONION:$P2P_PORT

  Add these to ~/.latticed/latticed.conf so the node listens locally and
  advertises the onion name rather than a private LAN address:

    listen=$LOCAL_ADDR:$P2P_PORT
    externalip=$ONION:$P2P_PORT
    onion=127.0.0.1:9050

  Deliberately no proxy= line. That routes every outbound dial through Tor,
  including loopback dials to a local peer, which Tor will not carry. onion=
  alone routes .onion traffic and leaves everything else direct.

  then: systemctl --user restart latticed

  Anyone else joining over Tor needs the onion= line plus:

    addpeer=$ONION:$P2P_PORT

  Publish that address wherever people look for the network. It is not secret —
  it is the whole point — but it is also not discoverable, so nobody will find
  it unless you put it somewhere.

DONE

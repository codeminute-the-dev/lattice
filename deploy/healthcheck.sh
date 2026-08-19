#!/usr/bin/env bash
# Check that a Lattice host came up complete.
#
# Written to be run after a reboot, because the failure that matters here is
# quiet: latticed starts fine with no peers, the CPU miner refuses to run
# without one, and nothing in the log says why the chain stopped growing.
#
#   bash deploy/healthcheck.sh
#
# Exit 0 if everything a running chain needs is present.
set -uo pipefail

CONF="${LATTICED_CONF:-$HOME/.latticed/latticed.conf}"
LATCTL="${LATCTL:-$HOME/Desktop/lattice/bin/latctl}"
RPC="${RPC:-127.0.0.1:44107}"
fail=0

ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$*"; fail=1; }
warn() { printf '  \033[33m!\033[0m %s\n' "$*"; }

echo
echo "Services"
for s in latticed latticed-peer latwallet latwalletgui latstatus latsite lattice-tunnel; do
  active=$(systemctl --user is-active "$s" 2>/dev/null)
  enabled=$(systemctl --user is-enabled "$s" 2>/dev/null)
  if [ "$active" = "active" ] && [ "$enabled" = "enabled" ]; then
    ok "$s (active, enabled)"
  else
    bad "$s (active=$active enabled=$enabled)"
  fi
done

# User units only start at boot when the account lingers; otherwise they wait
# for a login, which on a headless box may never come.
if [ "$(loginctl show-user "$USER" -p Linger --value 2>/dev/null)" = "yes" ]; then
  ok "linger enabled (units start at boot, not at login)"
else
  bad "linger disabled — run: loginctl enable-linger $USER"
fi

echo
echo "Tor"
if systemctl is-active tor >/dev/null 2>&1; then ok "tor running"; else bad "tor not running"; fi
if systemctl is-enabled tor >/dev/null 2>&1; then ok "tor enabled at boot"; else bad "tor not enabled at boot"; fi
if ss -ltn 2>/dev/null | grep -q '127.0.0.1:9050'; then ok "SOCKS on 9050"; else bad "no SOCKS listener on 9050"; fi

ONION=$(sed -n 's/^externalip=\(.*\):.*/\1/p' "$CONF" 2>/dev/null | head -1)
if [ -n "$ONION" ]; then
  ok "advertising $ONION"
else
  warn "no externalip= in $CONF — this node is not reachable by anyone"
fi

echo
echo "Chain"
U=$(sed -n 's/^rpcuser=//p' "$CONF" 2>/dev/null | head -1)
P=$(sed -n 's/^rpcpass=//p' "$CONF" 2>/dev/null | head -1)
q() { timeout 10 "$LATCTL" -u "$U" -P "$P" -s "$RPC" --notls "$@" 2>/dev/null; }

HEIGHT=$(q getblockcount)
PEERS=$(q getconnectioncount)
if [ -n "$HEIGHT" ]; then ok "height $HEIGHT"; else bad "node RPC not answering"; fi

# The load-bearing one. The CPU miner exits its loop while ConnectedCount()==0,
# so a node with no peers mines nothing and says nothing about it.
if [ "${PEERS:-0}" -gt 0 ] 2>/dev/null; then
  ok "$PEERS peer(s)"
else
  bad "0 peers — the CPU miner will not mine. Is latticed-peer up?"
fi

if grep -q '^generate=1' "$CONF" 2>/dev/null; then
  ok "mining enabled"
  if [ -n "$HEIGHT" ]; then
    sleep 8
    H2=$(q getblockcount)
    [ "${H2:-0}" -gt "${HEIGHT:-0}" ] 2>/dev/null && ok "chain advanced to $H2 during the check" \
      || warn "no new block in 8s (normal: blocks take ~40s)"
  fi
else
  warn "generate=0 — this node is not mining"
fi

echo
echo "Web"
for u in "http://127.0.0.1:8099/status" "http://127.0.0.1:8099/api/chain" "http://127.0.0.1:8081/" "http://127.0.0.1:8090/"; do
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$u")
  [ "$code" = "200" ] && ok "$u" || bad "$u returned $code"
done

PUB=$(curl -s -o /dev/null -w '%{http_code}' --max-time 20 https://lattice.codeminute.dev/api/chain)
[ "$PUB" = "200" ] && ok "public API (lattice.codeminute.dev)" || bad "public API returned $PUB"

echo
if [ "$fail" -eq 0 ]; then
  printf '\033[32mAll good.\033[0m\n\n'
else
  printf '\033[31mSomething is missing — see the ✗ lines above.\033[0m\n\n'
fi
exit "$fail"

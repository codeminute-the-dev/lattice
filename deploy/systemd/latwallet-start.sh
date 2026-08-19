#!/bin/sh
# Reads the node RPC credentials out of latticed.conf and execs latwallet with
# them. Keeping this in a wrapper rather than the unit file means the password
# never appears in `systemctl cat`, `ps`, or the journal.
set -eu
CONF="${LATTICED_CONF:-$HOME/.latticed/latticed.conf}"
USER_=$(sed -n 's/^rpcuser=//p' "$CONF" | head -1)
PASS_=$(sed -n 's/^rpcpass=//p' "$CONF" | head -1)
[ -n "$USER_" ] && [ -n "$PASS_" ] || { echo "no rpcuser/rpcpass in $CONF" >&2; exit 2; }
exec "$HOME/Desktop/lattice/bin/latwallet" \
  -u "$USER_" -P "$PASS_" \
  --noclienttls --noservertls \
  --rpcconnect=127.0.0.1:44107

#!/bin/sh
set -e

# Ensure hostname resolves to 127.0.0.1 to avoid warnings/errors
# This mimics the behavior of the GCE deployment script
if ! grep -q "$(hostname)" /etc/hosts; then
    echo "127.0.0.1 $(hostname)" >> /etc/hosts
fi

exec /usr/local/bin/dnsseeder "$@"

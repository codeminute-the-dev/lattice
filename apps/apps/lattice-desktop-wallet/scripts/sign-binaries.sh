#!/bin/bash

# Sign latwallet macOS binaries with Developer ID Application certificate
# This should be run before building the Electron app
#
# Note: Only macOS binaries need Apple code signing for notarization
# Linux and Windows binaries are not signed with Apple certificates

set -e

CERT_NAME="Developer ID Application"
BIN_DIR="$(dirname "$0")/../bin"

echo "Signing macOS latwallet binaries..."

# Sign x64 binary
if [ -f "$BIN_DIR/latwallet-darwin-x64" ]; then
  echo "Signing latwallet-darwin-x64..."
  codesign --force --sign "$CERT_NAME" \
    --options runtime \
    --timestamp \
    "$BIN_DIR/latwallet-darwin-x64"
  echo "✓ Signed latwallet-darwin-x64"
else
  echo "⚠ Warning: latwallet-darwin-x64 not found"
fi

# Sign arm64 binary
if [ -f "$BIN_DIR/latwallet-darwin-arm64" ]; then
  echo "Signing latwallet-darwin-arm64..."
  codesign --force --sign "$CERT_NAME" \
    --options runtime \
    --timestamp \
    "$BIN_DIR/latwallet-darwin-arm64"
  echo "✓ Signed latwallet-darwin-arm64"
else
  echo "⚠ Warning: latwallet-darwin-arm64 not found"
fi

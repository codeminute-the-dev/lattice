# macOS App Signing & Notarization Guide

## Overview
This guide explains how to sign and notarize the Lattice Wallet for macOS distribution.

## Prerequisites

### 1. Get an Apple developer account

### 2. Developer ID Application Certificate
- Created via https://developer.apple.com/account/resources/certificates
- Installed in Keychain with private key
- Verify: `security find-identity -v -p codesigning`

### 3. Environment Variables
Export before building (do not commit these):

```bash
export APPLE_ID="your-apple-id@email.com"
export APPLE_APP_SPECIFIC_PASSWORD="xxxx-xxxx-xxxx-xxxx"  # From appleid.apple.com
export APPLE_TEAM_ID="your-team-id"
```

## Build & Sign Process

### Step 1: Sign Native Binaries

Before building the Electron app, sign the `latwallet` Go binaries:

```bash
cd <PATH_TO_LATTICE>/lattice/apps/apps/lattice-desktop-wallet

bash scripts/sign-binaries.sh
```

**What it does:**
- Signs `bin/latwallet-darwin-x64` with Developer ID certificate
- Signs `bin/latwallet-darwin-arm64` with Developer ID certificate
- Enables hardened runtime (`--options runtime`)
- Adds secure timestamp

**Manual commands (if needed):**
```bash
codesign --force --sign "Developer ID Application" \
  --options runtime \
  --timestamp \
  bin/latwallet-darwin-x64

codesign --force --sign "Developer ID Application" \
  --options runtime \
  --timestamp \
  bin/latwallet-darwin-arm64
```

### Step 2: Build & Sign the Electron App

`electron-builder.json` is already configured correctly:
- `hardenedRuntime: true` — required by Apple
- `notarize: false` — notarization is done manually in Step 3 (so we don't block on Apple's review)
- `build/entitlements.mac.plist` — committed, no action needed

```bash
# Ensure environment variables are set
echo $APPLE_ID  # Should print your Apple ID

# Build and sign for macOS
pnpm run build:mac
```

**What happens:**
1. `electron-vite build` — builds renderer/main/preload code
2. `electron-builder --mac` — packages and signs both x64 and arm64:
   - Signs all app components with your Developer ID cert
   - Creates `dist/Lattice Wallet-0.0.1.dmg` (Intel)
   - Creates `dist/Lattice Wallet-0.0.1-arm64.dmg` (Apple Silicon)

### Step 3: Submit for Notarization (no waiting)

Apple notarization can take anywhere from minutes to days. Submit without blocking:

```bash
cd <PATH_TO_LATTICE>/lattice/apps/apps/lattice-desktop-wallet

# Submit x64 DMG
xcrun notarytool submit "dist/Lattice Wallet-0.0.1.dmg" \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_APP_SPECIFIC_PASSWORD" \
  --team-id "$APPLE_TEAM_ID" \
  --no-wait

# Submit arm64 DMG
xcrun notarytool submit "dist/Lattice Wallet-0.0.1-arm64.dmg" \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_APP_SPECIFIC_PASSWORD" \
  --team-id "$APPLE_TEAM_ID" \
  --no-wait
```

**Save the submission IDs** printed in the output — you'll need them to check status later.

### Step 4: Check Notarization Status

Come back later and check:

```bash
# Check all recent submissions
xcrun notarytool history \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_APP_SPECIFIC_PASSWORD" \
  --team-id "$APPLE_TEAM_ID"

# Check a specific submission by ID
xcrun notarytool info <submission-id> \
  --apple-id "$APPLE_ID" \
  --password "$APPLE_APP_SPECIFIC_PASSWORD" \
  --team-id "$APPLE_TEAM_ID"
```

Status will be `In Progress`, `Accepted`, or `Invalid`.

### Step 5: Staple the Notarization Ticket

Once status is `Accepted`, staple the ticket to both DMGs:

```bash
xcrun stapler staple "dist/Lattice Wallet-0.0.1.dmg"
xcrun stapler staple "dist/Lattice Wallet-0.0.1-arm64.dmg"
```

The DMGs are now fully signed, notarized, and ready for distribution.

### Step 6: Verify

```bash
# Verify app signature
codesign --verify --deep --strict --verbose=2 "dist/mac/Lattice Wallet.app"

# Verify notarization
spctl --assess --verbose=2 "dist/mac/Lattice Wallet.app"
# Should output: "source=Notarized Developer ID"

# Verify DMG
spctl -a -vvv -t install "dist/Lattice Wallet-0.0.1-arm64.dmg"
```

## Distributing to Users

### Option 1: Website Direct Download
Upload to CDN/hosting → users download → macOS verifies notarization → installs without warnings.

### Option 2: GitHub Releases

The CI/CD workflow automatically:
1. Builds and signs
2. Notarizes
3. Creates GitHub Release
4. Attaches DMGs as release assets

Users download from: https://github.com/codeminute-the-dev/lattice/releases

## Troubleshooting

### "skipped macOS code signing — identity explicitly is set to null"
- **Cause**: Old config had `identity: null` explicitly set
- **Solution**: Already fixed — `identity: null` has been removed from `electron-builder.json`

### "Code object is not signed"
- **Cause**: Binary wasn't signed before packaging
- **Solution**: Run `scripts/sign-binaries.sh` first

### "Notarize options were unable to be generated"
- **Cause**: Environment variables not set
- **Solution**: Export `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID`

### "Source=no usable signature"
- **Cause**: DMG created before .app was signed/stapled
- **Solution**: Rebuild DMGs after stapling

### Notarization rejected ("Invalid")
- **Check logs**:
  ```bash
  xcrun notarytool log <submission-id> \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_APP_SPECIFIC_PASSWORD" \
    --team-id "$APPLE_TEAM_ID"
  ```

## CI/CD (GitHub Actions)

The workflow automatically handles everything:

1. Builds Go binaries (latwallet)
2. Signs binaries
3. Builds Electron app
4. Signs app bundle
5. Submits for notarization (waits for approval)
6. Creates GitHub Release with signed installers

**Required GitHub Secrets:**
- `APPLE_ID`
- `APPLE_APP_SPECIFIC_PASSWORD`
- `APPLE_TEAM_ID`

Set environment in workflow:
```yaml
build:
  environment: internal  # or mainnet
  # Secrets are automatically available from the environment
```

## Quick Reference

| Step | Command | Purpose |
|------|---------|---------|
| 1. Sign binaries | `bash scripts/sign-binaries.sh` | Sign latwallet Go binaries |
| 2. Build & sign | `pnpm run build:mac` | Build and sign the app (no notarization wait) |
| 3. Submit | `xcrun notarytool submit ... --no-wait` | Submit both DMGs to Apple |
| 4. Check status | `xcrun notarytool history ...` | Monitor Apple's review |
| 5. Staple | `xcrun stapler staple "dist/..."` | Attach ticket once Accepted |
| 6. Verify | `spctl --assess --verbose=2 ...` | Confirm notarization |

## File Locations

After successful build:

```
dist/
├── Lattice Wallet-0.0.1.dmg          # Intel installer (signed, notarized after stapling)
├── Lattice Wallet-0.0.1-arm64.dmg    # Apple Silicon installer (signed, notarized after stapling)
├── mac/
│   └── Lattice Wallet.app            # Intel app bundle
└── mac-arm64/
    └── Lattice Wallet.app            # Apple Silicon app bundle
```

## First Launch Behavior

When users download and open for the first time:
- **30-second delay** on first launch (normal — Gatekeeper verification)
- **Instant launches** after that
- **No scary warnings** — just a confirmation dialog with your company name

## Resources

- [Apple Notarization Guide](https://developer.apple.com/documentation/security/notarizing_macos_software_before_distribution)
- [electron-builder Code Signing](https://www.electron.build/code-signing)
- [Apple System Status](https://developer.apple.com/system-status/)

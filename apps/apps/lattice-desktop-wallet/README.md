# Lattice Desktop Wallet

A modern, secure desktop wallet application for the Lattice blockchain network. Built with Electron, React, and TypeScript.

## Prerequisites

- **Go** >= 1.26.1
- **Rust** (stable toolchain) — required to build ZK proof libraries
- **Task** (`go-task`) — task runner for the backend build
- **Node.js** >= 22.0.0
- **pnpm** >= 8.15.6

## Setup

### 1. Build the latwallet binary (backend wallet daemon)

The desktop wallet depends on the `latwallet` binary from the root lattice repository. Build it from the repo root:

```bash
# From the lattice repo root
task build:blockchain
```

This compiles all blockchain binaries (including `latwallet`) into `lattice/bin/`.

> **Note:** `task build:blockchain` requires Rust (for ZK/XMSS FFI libraries) and CGO. Make sure your toolchain is set up before running this.

### 2. Copy latwallet to the wallet's bin directory

Copy the compiled binary to `apps/apps/lattice-desktop-wallet/bin/`, renaming it to match your OS and architecture:

| Platform          | Binary name              |
|-------------------|--------------------------|
| macOS (Apple Silicon) | `latwallet-darwin-arm64`  |
| macOS (Intel)     | `latwallet-darwin-x64`      |
| Linux (x64)       | `latwallet-linux-x64`       |
| Windows (x64)     | `latwallet-windows-x64.exe` |

**macOS (Apple Silicon):**
```bash
cp bin/latwallet apps/apps/lattice-desktop-wallet/bin/latwallet-darwin-arm64
```

**macOS (Intel):**
```bash
cp bin/latwallet apps/apps/lattice-desktop-wallet/bin/latwallet-darwin-x64
```

**Linux:**
```bash
cp bin/latwallet apps/apps/lattice-desktop-wallet/bin/latwallet-linux-x64
```

**Windows** (PowerShell):
```powershell
Copy-Item bin\latwallet.exe apps\apps\lattice-desktop-wallet\bin\latwallet-windows-x64.exe
```

### 3. Install frontend dependencies

```bash
# From lattice/apps
cd apps
pnpm install
```

## Development

Run the wallet in development mode (hot-reload):

```bash
# From lattice/apps
pnpm --filter @lattice/lattice-desktop-wallet dev

# Or from lattice/apps/apps/lattice-desktop-wallet
pnpm dev
```

## Building

Build the Electron app:

```bash
# From lattice/apps
pnpm --filter @lattice/lattice-desktop-wallet build

# Or from lattice/apps/apps/lattice-desktop-wallet
pnpm build
```

Build a distributable for your platform:

```bash
# macOS
pnpm build:mac

# Linux
pnpm build:linux

# Windows
pnpm build:win
```

Output is placed in `dist/`.

## Viewing Logs

```bash
# Tail live logs
pnpm logs

# Open logs directory in Finder (macOS)
pnpm logs:open
```

## Project Structure

```
lattice-desktop-wallet/
├── bin/                  # Platform-specific latwallet binaries (not committed)
├── src/
│   ├── main/             # Electron main process
│   ├── preload/          # Preload scripts
│   ├── renderer/         # React frontend
│   ├── types/            # Shared TypeScript types
│   └── utils/            # Shared utilities
├── resources/            # Static assets bundled with the app
├── electron.vite.config.ts
├── electron-builder.json
└── package.json
```

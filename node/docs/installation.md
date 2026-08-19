# Installation

## Prebuilt binaries (recommended)

The release installer downloads the platform archive from GitHub Releases,
verifies its SHA-256 against `checksums.txt`, and installs `latticed`, `latctl`,
`latwallet`, and `latwalletcli` (the interactive wallet CLI, in releases that
include it):

- macOS/Linux: `install.sh` → `${XDG_BIN_HOME:-$HOME/.local/bin}`
- Windows: `install.ps1` → `%LOCALAPPDATA%\Lattice\bin`

It also writes mainnet default configs into the OS default app-data paths when
missing, with shared auto-generated RPC credentials. Latwallet defaults to SPV
sync (`usespv=1`), so a local latticed is optional for the wallet. After install,
no `-u` / `-P` / `-C` is required: `latctl getinfo` targets local latticed, and
`latctl --wallet getinfo` targets local latwallet. Existing configs that already
have credentials are left unchanged. RPC stays localhost-only.

| Tool | Linux | macOS | Windows |
|------|-------|-------|---------|
| latticed | `~/.latticed/latticed.conf` | `~/Library/Application Support/Latticed/latticed.conf` | `%LOCALAPPDATA%\Latticed\latticed.conf` |
| latwallet | `~/.latwallet/latwallet.conf` | `~/Library/Application Support/Latwallet/latwallet.conf` | `%LOCALAPPDATA%\Latwallet\latwallet.conf` |
| latctl | `~/.latctl/latctl.conf` | `~/Library/Application Support/Latctl/latctl.conf` | `%LOCALAPPDATA%\Latctl\latctl.conf` |

Supported platforms: macOS and Linux on amd64 and arm64; Windows on amd64.

### macOS / Linux — download, inspect, then run

```bash
curl -fsSL -o install.sh https://raw.githubusercontent.com/codeminute-the-dev/lattice/main/install.sh
less install.sh
sh install.sh
```

### macOS / Linux — one-line convenience form

```bash
curl -fsSL https://raw.githubusercontent.com/codeminute-the-dev/lattice/main/install.sh | sh
```

### Windows — download, inspect, then run

```powershell
irm https://raw.githubusercontent.com/codeminute-the-dev/lattice/main/install.ps1 -OutFile install.ps1
notepad install.ps1
pwsh -File .\install.ps1
```

### Windows — one-line convenience form

```powershell
irm https://raw.githubusercontent.com/codeminute-the-dev/lattice/main/install.ps1 | iex
```

### Pin a version or install directory

```bash
sh install.sh --version v0.1.0
sh install.sh --bin-dir "$HOME/bin"
```

```powershell
.\install.ps1 -Version v0.1.0
.\install.ps1 -BinDir "$env:USERPROFILE\bin"
```

### Upgrade

Rerun the installer (with the same `--bin-dir` / `-BinDir` if you customized it).
Existing binaries are replaced atomically. Existing config files are left
unchanged.

### Remove

```bash
rm -f "${XDG_BIN_HOME:-$HOME/.local/bin}/latticed" \
      "${XDG_BIN_HOME:-$HOME/.local/bin}/latctl" \
      "${XDG_BIN_HOME:-$HOME/.local/bin}/latwallet" \
      "${XDG_BIN_HOME:-$HOME/.local/bin}/latwalletcli"
```

```powershell
Remove-Item "$env:LOCALAPPDATA\Lattice\bin\latticed.exe", `
            "$env:LOCALAPPDATA\Lattice\bin\latctl.exe", `
            "$env:LOCALAPPDATA\Lattice\bin\latwallet.exe", `
            "$env:LOCALAPPDATA\Lattice\bin\latwalletcli.exe" -ErrorAction SilentlyContinue
```

Configs are not removed automatically. Delete them from the paths in the table
above if you also want to discard RPC credentials and settings.

### macOS Gatekeeper / Windows SmartScreen

Installing via `curl`/`sh` or `irm` normally does not attach browser quarantine
/ Mark-of-the-Web the same way a browser download does, so Gatekeeper and
SmartScreen typically do not block the binaries. Archives downloaded in a
browser can still be blocked when the app is not platform-signed. The installer
never deletes quarantine metadata, Zone.Identifier streams, or otherwise
bypasses OS protections.

## Requirements (build from source)

- [Go](https://golang.org) 1.26 or newer
- [Rust](https://rustup.rs) toolchain (for ZK verification library)
- C compiler (for XMSS library)
- [Task](https://taskfile.dev) runner

## Build from Source

Clone the repository and build the blockchain binaries:

```bash
git clone https://github.com/codeminute-the-dev/lattice.git
cd lattice
task build:blockchain
```

Binaries are placed in `bin/`:
- `latticed` — full node
- `latctl` — CLI control tool
- `latwallet` — wallet daemon
- `latwalletcli` — interactive wallet CLI

To build only the node:

```bash
task build:latticed
```

## Startup

latticed will run and start downloading the block chain with no extra
configuration necessary. See the
[configuration documentation](configuration.md) for advanced options.

```bash
./bin/latticed
```

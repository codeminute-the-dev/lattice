# Lattice Gateway

A lightweight bridge between a Lattice full node and a local Miner process, providing low-latency work distribution and safe submission of candidate blocks.

## Overview

LatticeGateway sits between your Lattice node (e.g., latticed) and a local Miner process, exposing a minimal JSON-RPC surface to the Miner while managing the full Lattice Core RPC dialogue internally.

### Features

- Low-latency work distribution via dedicated memory cache
- Safe block submission with full template management
- JSON-RPC interface with validation using JSON Schema
- Support for Unix Domain Socket and TCP communication
- Configurable refresh intervals and authentication

### Architecture

```
                 Miner Process
             (JSON-RPC via UDS/loopback)

          ▲                        ▼
 getMiningInfo              submitPlainProof(PlainProof)
          │                        │
 ┌────────────────────────────────────────────┐
 │             LatticeGateway                   │
 │                                            │
 │ ┌────────────┐   ┌───────────────────────┐ │
 │ │ Miner RPC  │──►│      Work Cache       │ │
 │ │  Server    │   └───────────────────────┘ │
 │ └────────────┘               ▲             │
 │         │                    │             │
 │         ▼                    │             │
 │ ┌───────────────────┐  ┌─────────────────┐ │
 │ │  ProofGenerator   │──►│ Submission Svc │ │
 │ └───────────────────┘  └─────────────────┘ │
 └────────────────────────────────────────────┘
           │ getblocktemplate      │ submitblock
           ▼                       ▲
      LatticeNode Client (latticed)
```

### Data Flow

```mermaid
sequenceDiagram
    participant Miner
    participant AsyncLoopManager
    participant Gateway
    participant ProofGenerator
    participant LatticeNode

    Miner->>AsyncLoopManager: OpenedBlockInfo (A, B, A_noised, B_noised)
    AsyncLoopManager->>AsyncLoopManager: secondary_testing (uses noised)
    AsyncLoopManager->>AsyncLoopManager: create_proof (uses non-noised A, B)
    AsyncLoopManager->>Gateway: submitPlainProof(base64_string)
    Gateway->>ProofGenerator: generate_block(PlainProof)
    ProofGenerator->>LatticeNode: submit block
```


### Installation

To install the package from this directory:

```bash
# Standard installation
uv pip install .

# Development installation (includes test dependencies)
uv pip install -e . --group dev
```

The `zk-pow-rust` module will be automatically built and installed from the local `zk-pow` directory.

## Configuration

LatticeGateway uses environment variables for configuration with sensible defaults built-in. No configuration file is required.

### Configuration Options

All configuration can be set via environment variables:

**Lattice Node Connection:**
- `LATTICED_RPC_URL` - Lattice node RPC URL
- `LATTICED_RPC_USER` - RPC username (default: `user`)
- `LATTICED_RPC_PASSWORD` - RPC password (default: `pass`)
- `LATTICED_REFRESH_INTERVAL_SECONDS` - Template refresh interval (default: `1`)
- `LATTICED_MINING_ADDRESS` - Taproot address for block rewards (optional, overrides node's mining address)

**Miner RPC Server:**
- `MINER_RPC_TRANSPORT` - Transport type: `uds` or `tcp` (default: `uds`)
- `MINER_RPC_SOCKET_PATH` - Unix socket path (default: `/tmp/latticegw.sock`)
- `MINER_RPC_PORT` - TCP port for miners (default: `8337`)
- `MINER_RPC_HOST` - TCP host (default: `localhost`)

**Logging:**
- `LOGGING_LEVEL` - Log level: `debug`, `info`, `warning`, `error` (default: `info`)

## Usage

### Starting the Gateway

After installing the package, you can use the `lattice-gateway` command:

```bash
# Start with default configuration
lattice-gateway start

# Start with custom configuration via environment variables
export LATTICED_RPC_URL="https://my-lattice-node:44107"
export LATTICED_RPC_USER="myuser"
export LATTICED_RPC_PASSWORD="mypassword"
export LOGGING_LEVEL="debug"
# Start with a custom mining address (Taproot only)
export LATTICED_MINING_ADDRESS="bc1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr"
lattice-gateway start

# Enable verbose logging
lattice-gateway start -v
```

### Running Tests

The project uses pytest for testing:

```bash
# Run all tests
pytest
```



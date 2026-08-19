# Lattice (LATT)

Lattice is an L1 blockchain where mining is a by-product of **real matrix
multiplication**. Instead of burning electricity on arbitrary hashing, miners
perform integer matmul work and prove they did it correctly with a zero-knowledge
proof. The consensus mechanism — Proof-of-Useful-Work via NoisyGEMM, based on
[this paper](https://arxiv.org/abs/2504.09971) — is inherited unchanged from
[Pearl](https://github.com/pearl-research-labs/pearl), which Lattice forks.

**What Lattice changes is the money, not the cryptography.**

## The short version

| | |
|---|---|
| **Ticker** | LATT |
| **Block reward** | Starts at exactly 2 LATT |
| **Reward decay** | Smooth and gradual — 1.92 LATT after a year, no halvings |
| **Reset** | Every ~4 years the reward jumps back to 2 LATT and starts over |
| **Max supply** | None. Emission continues indefinitely by design |
| **Block time** | 40 seconds |
| **Premine** | None. Zero coins existed before the first mined block |
| **Founder allocation** | None |
| **Transaction fees** | Near-zero floor (10 cells/kB), free market above it |

The reset is the differentiator, and [`docs/emission.md`](docs/emission.md)
explains it in full. In brief: a pure decay curve eventually stops paying for the
security it buys, and Bitcoin's answer — assume fees replace the subsidy — is an
untested bet. Lattice instead restarts the curve on a fixed, published schedule,
so the security budget does not quietly evaporate.

The honest tradeoff: **Lattice is mildly inflationary, permanently.** It is not
*unpredictably* inflationary — every future reward is fixed at launch and
computable by anyone, offline, today.

Ask any node where the schedule stands:

```bash
latctl getnextreset
```

## Why solo mining works here

Fast blocks mean frequent, small, low-variance payouts. A miner with a modest
share of network hashrate lands blocks regularly instead of waiting months for
one large payout, which is what usually forces people into pools. A chain that
requires pools re-centralises the thing proof-of-work exists to decentralise, so
Lattice targets 40-second blocks to keep solo mining viable.

See [`docs/solo-mining.md`](docs/solo-mining.md) to get started.

## Fair launch

The genesis block pays **zero** to a provably unspendable `OP_RETURN`. There is
no premine, no founder allocation, no developer tax, and no pre-mined treasury.

You do not have to take that on faith:

```bash
# Regenerate the genesis blocks from source and diff against what ships.
go run ./node/cmd/gengenesis > /tmp/genesis.go
diff /tmp/genesis.go node/chaincfg/genesis.go && echo "genesis reproduces exactly"

# The consensus-level checks that enforce it.
go test ./node/chaincfg/ -run TestGenesisHasNoPremine -v
go test ./node/blockchain/ -run TestGenesisMintsNothing -v
```

The genesis coinbase message anchors the chain to Bitcoin block #963094, whose
hash did not exist before that block was mined — evidence the genesis was not
quietly mined ahead of its stated date.

## Anti-ASIC posture

Lattice does **not** claim ASIC resistance. That claim is not reliably
achievable, and the work function is deliberately shaped so commodity ML
accelerators are useful for mining — that is the point of *useful* work.

What Lattice does is keep the work function's tunable shape parameters isolated
and version-tagged in
[`zk-pow/src/work_function_params.rs`](zk-pow/src/work_function_params.rs), so
that *if* hardware concentration ever justifies retuning them, the change is a
small auditable diff rather than a rewrite. The process is social, not automatic,
and there is no key that can force it: see [`docs/anti-asic.md`](docs/anti-asic.md).

## Repository layout

| Directory | Description |
|-----------|-------------|
| [`node/`](node/) | **latticed** — full node, reference implementation |
| [`wallet/`](wallet/) | **latwallet** — HD wallet daemon, plus [**latwalletcli**](wallet/cmd/latwalletcli/) |
| [`spv/`](spv/) | Light client using compact block filters |
| [`zk-pow/`](zk-pow/) | ZK proof-of-work circuit and verifier (Rust, Plonky2/STARKy) |
| [`plonky2/`](plonky2/) | Plonky2 SNARK proving system (Rust, vendored) |
| [`xmss/`](xmss/) | XMSS post-quantum signatures (C + Go FFI) |
| [`lattice-blake3/`](lattice-blake3/) | Blake3 hashing utilities (Rust) |
| [`miner/`](miner/) | vLLM miner — GPU mining infrastructure (Python/CUDA) |
| [`dnsseeder/`](dnsseeder/), [`coredns-dnsseed/`](coredns-dnsseed/) | DNS seeders |
| [`apps/`](apps/) | Frontend applications (desktop wallet) |
| [`website/`](website/) | The public site at lattice.codeminute.dev |
| [`docs/`](docs/) | Emission, mining, and governance documentation |

## Prerequisites

- [Go](https://golang.org) 1.26 or newer
- [Rust](https://rustup.rs) toolchain
- A C compiler (for XMSS)
- [Task](https://taskfile.dev) runner
- [Python](https://python.org) 3.12 + [uv](https://docs.astral.sh/uv/) and a
  [CUDA toolkit](https://developer.nvidia.com/cuda-toolkit) — only for the GPU miner

## Building

```bash
task build:blockchain   # latticed, latctl, latwallet, latwalletcli -> bin/
```

Or without the Task runner:

```bash
cd zk-pow && cargo run --release --no-default-features --bin build_cache \
    src/circuit/v2_cache.bin src/v1/v1_cache.bin && cd ..
cd zk-pow/bindings/go && cargo build --release && cd ../../..
cd xmss && make && cd ..
go build -tags xmss,zkpow -o bin/latticed      ./node
go build -tags xmss,zkpow -o bin/latctl        ./node/cmd/latctl
go build -tags xmss,zkpow -o bin/latwallet     ./wallet
go build -tags xmss,zkpow -o bin/latwalletcli  ./wallet/cmd/latwalletcli
```

The `zkpow` build tag links the Rust proving library. Without it the node builds
but cannot mine or verify proofs.

## Networks

| Network  | RPC   | P2P   | Wallet | Address prefix | Epoch length |
|----------|-------|-------|--------|----------------|--------------|
| Mainnet  | 44107 | 44108 | 44207  | `lat1…`        | 3,153,600 blocks (~4 years) |
| Testnet  | 44109 | 44110 | 44209  | `tlat1…`       | 10,000 blocks (~4.6 days) |
| Testnet2 | 44111 | 44112 | 44211  | `tlat1…`       | 10,000 blocks |
| Simnet   | 18556 | 18555 | 18554  | `rlat1…`       | 200 blocks (~2.2 hours) |
| Regtest  | 18334 | 18444 | 18332  | `rlat1…`       | 200 blocks |

Test networks compress the same curve shape into short epochs so resets can be
observed in minutes rather than years.

## Testing

```bash
task test               # everything
go test -tags xmss,zkpow ./...          # Go suite
cd zk-pow && cargo test --release --no-default-features   # Rust suite
```

## Status

**Alpha, version 0.1.0.** There is no public network. Mainnet parameters are
defined but no mainnet is running, and the DNS seeds do not resolve. Run it on
simnet or testnet and dogfood it; do not treat LATT as having value.

## License

ISC, inherited from Pearl and btcd. See [LICENSE](LICENSE).

Lattice is a fork of [Pearl](https://github.com/pearl-research-labs/pearl) by
Pearl Research Labs, which was itself forked from
[btcd](https://github.com/btcsuite/btcd),
[btcwallet](https://github.com/btcsuite/btcwallet), and
[neutrino](https://github.com/lightninglabs/neutrino). Upstream copyright
notices are preserved throughout the source.

# GPU mining

Lattice mining is integer matrix multiplication (NoisyGEMM) plus a proof that you
did it. This page covers the GPU miner in
[`miner/lattice-gemm/ada/`](../miner/lattice-gemm/ada/), which runs on ordinary
NVIDIA gaming cards. The reference miner in `miner/vllm-miner/` needs
Hopper-class hardware (H100); this one does not.

Status first, so nobody is surprised: this is alpha software on a chain with no
public peers and no market value. Everything here runs against your own node.

## Which cards work

| Card generation | Compute capability | Kernel | Notes |
|---|---|---|---|
| Ampere, Ada, Blackwell (RTX 30/40/50) | 8.0 and up | tensor cores | fastest path |
| Turing (RTX 20, GTX 16xx) | 7.5 | dp4a | GTX 16xx have no tensor cores at all |
| Pascal (GTX 10xx) | 6.1 | dp4a | needs the `cu126` PyTorch wheels |
| Maxwell and older | below 6.1 | none | not supported |
| Hopper (H100) | 9.0 | tensor cores, or `vllm-miner` | the reference kernels target this card |

Tested on real hardware: Ada (RTX 4060 Ti) and Turing (GTX 1650). The code is
compiled for Pascal, Ampere, Ada and every other row, and the same test suite
applies, but I have not run it on those cards. Report what you find.

The kernel is chosen automatically. `LATTICE_ADA_CONFIG=1` or `2` forces the
tensor core or dp4a kernel.

Both kernels are bit-exact against the Python reference (the test suite checks
every transcript word), and blocks they mine are accepted by the same Rust
verifier the node uses.

## Measured speed

Work is counted in tile hashes: one hash of a 16x16 block of the result matrix.
These are search rates on the machines I had, not block rates; see
[what speed means here](#what-speed-means-on-this-chain).

| Card | Kernel | Matrix multiply alone | Full mining attempt |
|---|---|---|---|
| RTX 4060 Ti (8 GB) | tensor cores | about 37M tile hashes/s (39 TOPS) | 42M tile hashes/s at 8192x8192x2048 |
| RTX 4060 Ti (8 GB) | dp4a | 14 to 20M tile hashes/s (15 to 21 TOPS) | not measured |
| GTX 1650 (4 GB) | dp4a | 4.5M tile hashes/s (4.7 TOPS) at 4096x4096x2048 | not measured |
| CPU, 6 threads, PyTorch reference | | about 0.02M tile hashes/s | |

The 4060 Ti numbers were taken while another process was also using the GPU, so
a quiet card should do somewhat better. The dp4a kernel is correct but has not
been tuned.

## Install

You need a clone of this repo plus [uv](https://docs.astral.sh/uv/), a Rust
toolchain, the CUDA toolkit (`nvcc` 12.0 or newer) and a C compiler.

```bash
miner/lattice-gemm/ada/install.sh              # venv in ~/.lattice-miner/venv
miner/lattice-gemm/ada/install.sh /path/to/venv
```

It creates the virtualenv, installs the right PyTorch build for your oldest GPU
(`TORCH_INDEX=cu126|cu128|cu130` overrides the choice), builds the proving
library, builds the CUDA library for sm_61 through sm_89, and prints which kernel
your card will use. The first run is slow because it compiles Rust.

Check it:

```bash
cd miner/lattice-gemm/ada
PYTHONPATH=. ~/.lattice-miner/venv/bin/python -m pytest -q --noconftest \
    -c /dev/null --rootdir . test_pow_gemm.py test_end_to_end.py -p no:cacheprovider
```

## Run

You need a running node (see [solo mining](solo-mining.md)) and a mining address.

```bash
export LATTICED_RPC_URL=http://127.0.0.1:44107
export LATTICED_RPC_USER=...          # from ~/.latticed/latticed.conf
export LATTICED_RPC_PASSWORD=...
export LATTICED_MINING_ADDRESS=...
export MINER_RPC_SOCKET_PATH=/tmp/latticegw.sock

~/.lattice-miner/venv/bin/lattice-gateway start &
cd miner/lattice-gemm/ada
PYTHONPATH=. ~/.lattice-miner/venv/bin/python -u mine.py          # GPU 0
PYTHONPATH=. ~/.lattice-miner/venv/bin/python -u mine.py --device 1   # a second card
```

`--device` uses `nvidia-smi` numbering. Run one process per GPU. `--m`, `--n`
and `--k` set the matrix shape (default 2048x2048x2048; `k` must be a multiple of
64 and at least 2048).

### As services

To keep mining across reboots, use the systemd units in
[`deploy/systemd/`](../deploy/systemd/):

```bash
mkdir -p ~/.config/lattice ~/.config/systemd/user
cp deploy/systemd/lattice-miner.env.example ~/.config/lattice/miner.env
chmod 600 ~/.config/lattice/miner.env      # edit it: paths, RPC login, address
cp deploy/systemd/lattice-gateway.service deploy/systemd/lattice-miner@.service \
   ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now lattice-gateway lattice-miner@0
journalctl --user -u lattice-miner@0 -f
```

Add `lattice-miner@1` for a second card.

## What speed means on this chain

Difficulty is set by the chain, not by your hardware. On a young chain with
almost no hashrate, difficulty sits at its floor (`PowLimit`), where a single tile
hash succeeds about one time in sixteen. A modest GPU then finds a block on
nearly every attempt and the miner idles until the next template. A busy-looking
GPU is not the goal: `nvidia-smi` will show low utilisation, and the log line
reports how busy the card actually was.

Difficulty rises on its own, one block at a time, only when blocks arrive faster
than the 40-second target (see `node/blockchain/difficulty.go`). More hashrate on
the network is what moves it.

## Troubleshooting

**`Lattice is downloading blocks...` from the node.** The node refuses to hand
out block templates when its newest block is more than 24 hours old, because it
assumes it is still syncing. On an idle chain that stops all mining. Set
`allowstaletemplates=1` in `latticed.conf` and restart. It only changes what the
local RPC does; it has no effect on consensus or on what peers see.

**`GPU too old`.** The card is below compute capability 6.1.

**PyTorch says the architecture is not supported.** Newer PyTorch builds drop
old architectures. Pascal cards should use `TORCH_INDEX=cu126` and Blackwell cards
`cu128` or newer; both are what I expect to work, neither is tested here.

**`waiting for the gateway`.** `mine.py` waits for the gateway socket. Check that
`lattice-gateway` is running and that `MINER_RPC_SOCKET_PATH` matches in both.

**Blocks stop at height 394,200 on mainnet.** That is where certificate V4
starts. The Ada miner already handles it; the Hopper `vllm-miner` kernels do not
yet.

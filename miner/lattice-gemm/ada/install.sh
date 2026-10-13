#!/usr/bin/env bash
# Install the Lattice GPU miner for NVIDIA cards from Pascal (GTX 10xx) up.
#
#   miner/lattice-gemm/ada/install.sh [venv-dir]
#
# Needs: uv, a Rust toolchain (cargo), nvcc >= 12.0, a C compiler, git checkout
# of this repo. Takes a while the first time (it builds the proving library).
#
# Environment:
#   VENV         virtualenv location (default ~/.lattice-miner/venv, or the first argument)
#   TORCH_INDEX  PyTorch wheel index suffix: cu126 (Pascal and newer, not
#                Blackwell), cu128, cu130 (Turing and newer). Default: picked
#                from the oldest GPU in the machine.
#   CARGO_TARGET_DIR, UV_CACHE_DIR, ...  honoured as usual, so heavy build
#                output can live on a bigger disk.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"
VENV="${1:-${VENV:-$HOME/.lattice-miner/venv}}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1 ($2)" >&2; exit 1; }; }
need uv    "https://docs.astral.sh/uv/"
need cargo "https://rustup.rs"
need nvcc  "CUDA toolkit 12.0 or newer"
need nvidia-smi "NVIDIA driver"

nvcc_major="$(nvcc --version | sed -n 's/.*release \([0-9]*\)\..*/\1/p')"
if [ "${nvcc_major:-0}" -lt 12 ]; then
  echo "nvcc $nvcc_major is too old, need >= 12.0" >&2
  exit 1
fi

# Oldest compute capability decides the PyTorch build.
min_cc="$(nvidia-smi --query-gpu=compute_cap --format=csv,noheader | tr -d '.' | sort -n | head -1)"
if [ -z "$min_cc" ]; then echo "no NVIDIA GPU found" >&2; exit 1; fi
if [ "$min_cc" -lt 61 ]; then
  echo "GPU compute capability is below 6.1 (older than GTX 10xx); not supported" >&2
  exit 1
fi
if [ -z "${TORCH_INDEX:-}" ]; then
  if [ "$min_cc" -lt 75 ]; then TORCH_INDEX=cu126; else TORCH_INDEX=cu130; fi
fi
echo "venv: $VENV   torch wheels: $TORCH_INDEX   oldest GPU: cc ${min_cc:0:1}.${min_cc:1}"

uv venv --python 3.12 "$VENV"
PY="$VENV/bin/python"
uvp() { uv pip install --python "$PY" "$@"; }

uvp torch --index-url "https://download.pytorch.org/whl/$TORCH_INDEX"
uvp maturin blake3 numpy aiohttp bitcoin-utils fastjsonschema prometheus-client \
    pybase64 "pydantic>=2.12.5" "pydantic-settings>=2.12.0" pyyaml loguru pytest

# The proving library (Rust). Its circuit caches are generated, not checked in.
if [ ! -s "$REPO/zk-pow/src/circuit/v2_cache.bin" ] || [ ! -s "$REPO/zk-pow/src/v1/v1_cache.bin" ]; then
  (cd "$REPO/zk-pow" && cargo run --release --no-default-features --bin build_cache \
      src/circuit/v2_cache.bin src/v1/v1_cache.bin)
fi
WHEELS="$(mktemp -d)"
trap 'rm -rf "$WHEELS"' EXIT
(cd "$REPO/py-lattice-mining" && "$VENV/bin/maturin" build --release --out "$WHEELS" -i "$PY")
uvp "$WHEELS"/py_lattice_mining-*.whl

# The miner packages pin an exact torch; the one above is what the GPU needs.
uvp --no-deps "$REPO/miner/miner-utils" "$REPO/miner/miner-base" "$REPO/miner/lattice-gateway"

make -C "$HERE"
(cd "$HERE" && PYTHONPATH=. "$PY" - <<'PYEOF'
import lattice_ada, torch
print("GPU:", torch.cuda.get_device_name(0), "| kernel:", {1: "tensor cores", 2: "dp4a"}[lattice_ada.best_config()])
PYEOF
)
echo
echo "Installed. Run the self-test with:"
echo "  cd $HERE && PYTHONPATH=. $PY -m pytest -q --noconftest -c /dev/null --rootdir . test_pow_gemm.py test_end_to_end.py -p no:cacheprovider"
echo "Mine with (node + gateway running, see docs/gpu-mining.md):"
echo "  cd $HERE && PYTHONPATH=. $PY -u mine.py"

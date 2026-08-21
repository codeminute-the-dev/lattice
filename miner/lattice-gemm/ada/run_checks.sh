#!/usr/bin/env bash
# Build and run every check for the Ada (sm_89) backend.
#
# Two of them are host programs and run anywhere nvcc is installed; the rest
# need a GeForce card. With no GPU present the script still builds all of them,
# which is worth doing on its own -- register pressure and shared-memory
# footprint are compile-time facts, and both are tight enough on Ada to be worth
# watching.
#
#   ./ada/run_checks.sh            from miner/lattice-gemm
#   CUDA_HOME=/opt/cuda ./ada/run_checks.sh

set -uo pipefail

cd "$(dirname "$0")/.."
ROOT=$(pwd)
CUTLASS="$ROOT/third_party/cutlass"
OUT=${OUT:-/tmp/lattice-ada-checks}

NVCC=${NVCC:-${CUDA_HOME:+$CUDA_HOME/bin/nvcc}}
NVCC=${NVCC:-$(command -v nvcc)}

if [ -z "${NVCC:-}" ] || [ ! -x "$NVCC" ]; then
  echo "nvcc not found. Set CUDA_HOME or NVCC." >&2
  exit 1
fi
if [ ! -d "$CUTLASS/include" ]; then
  echo "CUTLASS is missing. Run:" >&2
  echo "  git submodule update --init --depth 1 $CUTLASS" >&2
  exit 1
fi

mkdir -p "$OUT"

FLAGS=(-std=c++20 -O3 -w --expt-relaxed-constexpr --expt-extended-lambda -DNDEBUG)
INCLUDES=(-I "$CUTLASS/include" -I "$CUTLASS/tools/util/include" -I csrc -I ada)

# A full toolkit install puts libcudart where the linker already looks; a
# redistributable or conda one does not, so point at it explicitly when it is
# somewhere findable from nvcc.
LIBDIRS=()
NVCC_PREFIX=$(cd "$(dirname "$NVCC")/.." && pwd)
for d in lib64 lib targets/x86_64-linux/lib; do
  [ -d "$NVCC_PREFIX/$d" ] && LIBDIRS+=(-L "$NVCC_PREFIX/$d")
done

echo "nvcc:    $("$NVCC" --version | tail -1)"
echo "cutlass: $(git -C "$CUTLASS" describe --tags --always 2>/dev/null || echo unknown)"
if command -v nvidia-smi >/dev/null 2>&1 && nvidia-smi -L >/dev/null 2>&1; then
  HAVE_GPU=1
  echo "gpu:     $(nvidia-smi --query-gpu=name --format=csv,noheader | head -1)"
else
  HAVE_GPU=0
  echo "gpu:     none -- GPU checks will be built but not run"
fi
echo

failed=0
skipped=0

# build <name> <source> <arch> ; sets BIN
build() {
  local name=$1 src=$2 arch=$3
  printf '%-26s ' "$name"
  if ! "$NVCC" "${FLAGS[@]}" -arch="$arch" "${INCLUDES[@]}" ${LIBDIRS[@]+"${LIBDIRS[@]}"} \
        "$src" -o "$OUT/$name" > "$OUT/$name.log" 2>&1; then
    echo "BUILD FAILED  (see $OUT/$name.log)"
    failed=$((failed + 1))
    return 1
  fi
  return 0
}

# run <name> <needs_gpu>
run() {
  local name=$1 needs_gpu=$2
  if [ "$needs_gpu" = 1 ] && [ "$HAVE_GPU" = 0 ]; then
    echo "built, skipped (no GPU)"
    skipped=$((skipped + 1))
    return
  fi
  if "$OUT/$name" > "$OUT/$name.out" 2>&1; then
    echo "PASS"
  else
    echo "FAIL          (see $OUT/$name.out)"
    failed=$((failed + 1))
  fi
}

check() {  # check <name> <source> <arch> <needs_gpu>
  build "$1" "$2" "$3" || return
  run "$1" "$4"
}

echo "host checks"
check layout_equiv   ada/layout_equiv_sm89.cu  sm_89   0
check smem_budget    ada/smem_budget_sm89.cu   sm_90a  0

echo
echo "device checks"
check noisy_gemm     ada/noisy_gemm_sm89_test.cu         sm_89 1
check noising        ada/noising_sm89_test.cu            sm_89 1
check merkle_roots   ada/merkle_tree_roots_sm89_test.cu  sm_89 1

echo
echo "mainloop register pressure (compile-time; Ada allows 255)"
"$NVCC" "${FLAGS[@]}" -arch=sm_89 --use_fast_math \
  --ptxas-options=--verbose,--register-usage-level=10,--warn-on-local-memory-usage \
  "${INCLUDES[@]}" -c ada/regpressure_sm89.cu -o /dev/null 2>&1 |
  awk '/Used [0-9]+ registers/ { match($0, /Used [0-9]+ registers/); r = substr($0, RSTART, RLENGTH) }
       /spill stores/            { match($0, /[0-9]+ bytes spill stores/); s = substr($0, RSTART, RLENGTH) }
       END { print "  " r ", " s }'

echo
if [ "$failed" -eq 0 ]; then
  if [ "$skipped" -gt 0 ]; then
    echo "PASS: everything built; $skipped device check(s) still need a GeForce card"
  else
    echo "PASS: all checks"
  fi
  exit 0
fi
echo "FAIL: $failed check(s)"
exit 1

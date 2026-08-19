# Ada (sm_89) backend

The production kernels in `../csrc/` target **sm_90a** — Hopper. They are not
merely compiled for it; they are built on hardware that only Hopper has:

| Construct | Uses | Ada equivalent |
|---|---|---|
| `cute::SM90_TMA_LOAD` / `_STORE` | 30 | `cp.async` (`SM80_CP_ASYNC_*`) |
| `GMMA::ss_op_selector` (warpgroup MMA, smem operands) | 60 | `SM80_16x8x32_S32S8S8S32_TN` + `ldmatrix` |
| thread-block clusters | 92 | none — single CTA |
| `cutlass::PipelineTmaAsync` | — | `cutlass::PipelineAsync` |
| `SM90_TMA_REDUCE_ADD` | — | `atomicAdd` or a separate pass |

TMA is a physical unit introduced in Hopper. Ada (sm_89) does not have it, so
recompiling with a different `-gencode` moves the failure from link time to
compile time rather than fixing anything.

## Shared memory forces a retune too

This is not only an instruction port. The default matmul config is
`128x256x128` with 3 pipeline stages:

    A per stage 16 KB + B per stage 32 KB, x3 stages = 144 KB

Hopper allows 228 KB of shared memory per SM; Ada allows 100 KB per block. The
production tiling does not fit. Workable Ada candidates:

    128x128x64 x3 = 48 KB
    128x128x64 x4 = 64 KB
    128x64x64  x3 = 36 KB

Smaller K-depth per stage means more mainloop iterations, so the epilogue and
tile scheduler have to follow the mainloop rather than being ported verbatim.

## What already works unchanged

Six kernels use no Hopper constructs at all and need no porting: `blake3`,
`noise_generation`, `denoise_converter`, `inner_hash`, `tensor_hash`, and
`build_routing_data`. `kernel_traits.hpp` already loads scales through
`SM80_CP_ASYNC_CACHEALWAYS`, so there is an existing `cp.async` path in-tree to
model the TMA replacements on.

## smoke_sm89.cu

Establishes the premise the whole port depends on: that an SM80-class int8
tensor-core GEMM — the arithmetic the NoisyGEMM mainloop performs — runs
correctly on Ada. It checks results against a CPU reference, because "it ran"
and "it is correct" are different claims.

Run it:

    docker run --rm --gpus all -v "$PWD/..":/w -w /w \
      nvidia/cuda:12.6.3-devel-ubuntu24.04 sh -c '
        nvcc -std=c++17 -arch=sm_89 -O2 -w --expt-relaxed-constexpr \
          -I third_party/cutlass/include \
          -I third_party/cutlass/tools/util/include \
          ada/smoke_sm89.cu -o /tmp/smoke && /tmp/smoke'

Verified on a GeForce RTX 4060 Ti (AD106, sm_89), CUDA 12.6.3, CUTLASS v3.9.2:

    int8 GEMM 512x512x256 on sm_89: checked 182 outputs, 0 mismatched
    PASS: SM80 int8 tensor-core path is correct on this GPU

## Correctness bar

Mining is consensus-critical. A kernel that is subtly wrong does not error — it
produces proofs the verifier rejects, which presents as "mining silently does
not work". Every ported op must be validated against the reference
implementations in `../src/lattice_gemm/testing/` before it is trusted.

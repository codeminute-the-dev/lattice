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

TMA is a physical unit introduced in Hopper. Ada (sm_89) does not have it.

It is worth being precise about what happens if you just rebuild for sm_89,
because the answer is not what it looks like: **it compiles.** Every one of
those sources builds clean under `-arch=sm_89`, with or without `-DNDEBUG`.
cute guards its Hopper-only paths with `CUTE_INVALID_CONTROL_PATH`, which is a
runtime `assert(0)` plus a `printf` -- not a compile-time error -- and the
project builds with `-DNDEBUG`, which compiles the assert out and leaves only
the printf. On top of that `make_tma_copy` builds its descriptor on the host
through `cuTensorMapEncodeTiled`, which fails on a non-Hopper device, and the
cluster launch fails too.

So a successful build for sm_89 tells you nothing. This is the failure mode the
correctness bar at the bottom of this file is about: not an error, just wrong
answers.

## The constraint that shapes everything: the tile is consensus-visible

The obvious way to fit Ada's smaller shared memory is to shrink the tile. That
is not available here.

Every MMA thread runs an independent PoW attempt over the C elements held in
**its own registers**: `TileHashAccumulator` (`../csrc/gemm/pow_utils.hpp`)
XOR-folds the thread's accumulator fragment once every `R / 32` k-blocks into a
16-word BLAKE3 message block, and `check_pow_target` races that transcript
against the target. When a thread wins, `write_host_signal_header` records the
tile-relative rows and columns it held. Those become the proof's index lists
(`lattice_gemm/helpers.py::extract_indices`), which the verifier parses as a
periodic pattern (`zk-pow/src/v1/api/plain_proof.rs::list_to_pattern`) and
publishes as `MiningConfiguration.rows_pattern` / `cols_pattern`.

So the accumulator's thread layout *is* the definition of what a nonce commits
to. `miner/miner-base/src/miner_base/settings.py` states the production one
directly:

    tile_size_m = 128, tile_size_n = 256
    rows_pattern = [0, 8]
    cols_pattern = [0, 1, 8, 9, 16, 17, ..., 248, 249]

Two rows and sixty-four columns per thread — 128 accumulator registers, the
warpgroup-MMA C layout. Change bM or bN and every thread's row/column set
changes with it.

### The Ada tiling that reproduces it exactly

Hopper splits bM=128 across two warpgroups of 64 rows; the 4 warps inside each
warpgroup then take 16 rows apiece. Ada has no warpgroups, so the same split is
expressed directly — one `SM80_16x8x32_S32S8S8S32_TN` atom per warp, eight
warps laid out along M:

```cpp
using TiledMma = decltype(make_tiled_mma(
    SM80_16x8x32_S32S8S8S32_TN{}, Layout<Shape<_8, _1, _1>>{}));
```

This is not merely similar to the Hopper layout, it is the same map. Hopper
thread `128g + 32w + l` and Ada thread `32W + l` land on the same rows because
`64g + 16w == 16(4g + w)`. `layout_equiv_sm89.cu` checks all 256 threads and all
128 registers of each and finds zero coordinate mismatches, so the transcript,
the submitted row/column patterns, and the set of valid nonces are unchanged.

The atom's K extent is 32, the same as the warpgroup atom's, which keeps the
reduction cadence `R / 32` intact.

**bM and bN are therefore fixed at 128 and 256.** Only `bK` and the pipeline
depth are free.

## Shared memory: the denoise factors are the blocker, not A/B

Ada allows 101376 B per block against Hopper's 233472 B. Measuring the real
layouts rather than estimating them (`smem_budget_sm89.cu`, which reads them
straight out of `KernelTraits`) at the production tile and rank 128:

    A+B staging (128x256x128, 3 stages)   144.0 KB
    C                                      64.0 KB
    denoise EAL x EARxBpEB                 96.0 KB
    denoise AxEBL x EBR                    96.0 KB
    scales                                  1.5 KB
    -----------------------------------------------
    total                                 193.6 KB

A/B staging is the part everyone looks at, and it is not the binding
constraint: `bK=64` with 4 stages costs 96 KB, and dropping to 2 stages at
`bK=128` costs the same. The binding constraint is the 192 KB of denoise
factors, which no choice of `bK` touches at all.

That 192 KB is avoidable. `SharedStorageDenoise` holds all four factors at
once, but `collective_epilogue.hpp::denoise()` consumes them as two strictly
sequential GEMMs — `EAL x EARxBpEB` completes and releases its pipeline before
`AxEBL x EBR` is even waited on. Putting the two operand pairs in a union
instead of a struct costs the overlap between them and halves the requirement
to 96 KB.

With that regrouping:

    bK=64, 4 stages, two-phase denoise:  99936 B of 101376 B (98.6% used)

It fits, with 1440 B spare. `bK=128` with 2 stages fits identically and leaves
the k-blocking untouched; `bK=64` is preferred because cp.async has no TMA to
hide load latency behind, so a deeper pipeline at finer granularity is worth
more than the larger k-step. Either is safe: `layout_equiv_sm89.cu` also
simulates `TileHashAccumulator`'s bookkeeping and confirms the transcript
schedule at `bK=64` and `bK=32` is identical, reduction for reduction, to the
one at `bK=128`.

One consequence for later: `heuristics.hpp::get_pipeline_stages` adds the C
buffer on top of the A/B stages, but `SharedStorageDenoise` unions them. On
Hopper the 64 KB of slack only costs a stage or two; on Ada it is the
difference between 4 stages and 1, so the heuristic has to learn about the
union before the denoise path is usable.

## Registers: 202 of 255, nothing spills

The consensus-fixed accumulator is 128 int32 registers per thread, and that
number is not negotiable either. Hopper affords it because warpgroup MMA takes
both operands from shared memory as descriptors and holds no operand registers
at all. Ada's SM80 atom has to stage A and B through `ldmatrix`, on top of those
128 — so it is worth knowing whether the mainloop fits before writing it, since
a spilling int8 GEMM is not a port worth having.

`regpressure_sm89.cu` builds the mainloop's register working set — the full
accumulator, `ldmatrix` staging of both operands, 32 MMA instructions per
k-block, and the transcript fold at the production cadence — and lets `ptxas`
report on it, with the same flags `setup.py` passes:

    Used 202 registers, 0 bytes spill stores, 0 bytes spill loads
    64 bytes stack frame

It fits. The 64-byte stack frame is the 16-word transcript, indexed by a running
counter rather than a constant, so it lives in local memory on Hopper too; it is
touched once per k-tile, not in the inner loop.

At 202 registers x 256 threads Ada runs one CTA per SM, which the 96 KB of
shared memory would have forced regardless. The production kernel already runs
that shape — it declares `__launch_bounds__(..., 1)`, and 193.6 KB leaves no
room for a second CTA in Hopper's 228 KB. So this is the expected operating
point, not a regression, but there is no register headroom left to spend on
unrolling the mainloop further.

## What is ported

The GEMM path is written and compiles for sm_89 with no spills:

| Hopper | Ada |
|---|---|
| `collective_mainloop.hpp` | `collective_mainloop_sm89.hpp` |
| `collective_epilogue.hpp` | `collective_epilogue_sm89.hpp` |
| `tile_scheduler.hpp` (`SingleTileScheduler`) | `tile_scheduler_sm89.hpp` |
| `lattice_gemm_kernel.h` + `lattice_gemm_host.h` | `lattice_gemm_sm89.h` |
| `lattice_noisingA_kernel.h` + `lattice_noisingB_kernel.h` | `noising_kernel_sm89.hpp` |
| `tensor_hash/merkle_tree_roots_kernel.hpp` | `merkle_tree_roots_sm89.hpp` |

The assembled GEMM uses 252 registers of 255 and 100352 B of shared memory of
the 101376 available; the noising kernel 141 registers and 73728 B; the
Merkle-roots kernel 72 registers. Nothing spills.

Three things came out differently from a literal translation:

**No warp specialisation.** Hopper dedicates a warpgroup to issuing TMA and two
more to MMA, with three mbarrier pipelines between them. cp.async has no such
asymmetry — every thread loads for itself — so the CTA is exactly the 256 MMA
threads and the pipeline is the ordinary SM80 multistage one. Most of the
Hopper kernel's length is warp specialisation, and it disappears rather than
being ported.

**The denoise k-loop is deliberately not unrolled.** Each iteration's B fragment
is 64 registers, so an eight-deep unroll keeps 512 of them live next to a
128-register accumulator. That spilled 780 bytes; not unrolling it, and slicing
the k-block out of shared memory before building the fragments rather than
after, brings it to zero.

**Scale, convert and stage are one pass.** The Hopper epilogue converts the
whole fragment to bfloat16 before its stmatrix, which means holding 128 floats
and 128 bfloat16s at once. stmatrix is sm_90 anyway, so the Ada epilogue writes
each element into shared memory as it scales it, and saves the 64 registers.

**One noising kernel, not two.** `lattice_noisingA_kernel.h` and
`lattice_noisingB_kernel.h` are the same kernel written twice under different
names: A -> B, EAL -> EBR, EAR -> EBL, EBL -> EAR, ApEA -> BpEB,
AxEBL -> EARxBpEB. Each produces a noised matrix and a rank-R denoise factor,
and the one substantive difference is which matrix the factor multiplies --
noisingA the raw input, noisingB the output it has just noised. That is a
template parameter here.

Those kernels are warp-specialised three ways, with a pipeline whose job is to
carry BpEB from the warpgroup that produces it to the warpgroup that multiplies
it. Serialising the two consumers turns that dependency into a barrier, and
cp.async removes the producer warpgroup, so four pipelines become none.
Split-K's `SM90_TMA_REDUCE_ADD` becomes `atomicAdd`.

Two conversions there are easy to get wrong because they differ: the noise term
saturates on the way down from int32 (`cvt.pack.sat.s8.s32`), and the input is
then added in int8, which wraps. With 7-bit inputs at rank 128 the sum really
does leave int8's range, so a port that saturated both or wrapped both would be
wrong on real data.

**The Merkle-roots kernel loses its dual-pipeline mode.** One thread hashes one
1 KiB chunk, so a warp wants 32 chunks 1 KiB apart -- an access pattern that
reads terribly from global memory, which is why the Hopper kernel stages it
through shared memory with TMA. cp.async solves the same problem without a
producer warpgroup: the threads cooperatively load a contiguous slab covering
everyone's current slice into the same layout. The dual-pipeline mode existed
only because a TMA descriptor dimension cannot exceed 256, so 512 consumers
needed two descriptors, two pipelines and a second copy of the consumer loop;
cp.async has no such limit, and it collapses back to one.

That kernel's shared-memory budget does not survive the move, though. Its
Hopper default -- 256 threads staging 128-byte slices three deep -- wants
106496 B, and Ada allows 101376. Two stages at 128 bytes, or four at 64, fit;
512 threads fit only at 64 bytes and two stages. A `static_assert` rejects the
combinations that do not, so the host's runtime dispatch over stage counts has
to be narrowed on Ada rather than discovering this at launch.

### Alignment: cp.async is stricter than TMA

TMA takes its bounds from a descriptor and copes with any stride. cp.async and
the vector store do not — a 16-byte access must be 16-byte aligned — so each row
of A, B, C and the denoise factors has to start on a 16-byte boundary. Row
*counts* are still free, since M and N are predicated and the ZFILL variant of
cp.async writes zeros past the end exactly as TMA would; it is the row pitches
that are constrained:

    K % 16 == 0     (int8 A and B)
    N % 8  == 0     (bfloat16 C)
    R % 8  == 0     (fp16 denoise factors)

`run_lattice_gemm_sm89` rejects a problem that violates these rather than
issuing a misaligned access.

## What is left

Every kernel is ported. What remains is build and dispatch work -- nothing on
Ada reaches these kernels yet:

| Unit | Notes |
|---|---|
| `lattice_gemm_api.cpp` | nothing dispatches to the Ada kernels yet |
| `tensor_hash_host.hpp` | its stage-count dispatch offers combinations that do not fit Ada |
| `heuristics.hpp::get_pipeline_stages` | adds C on top of the A/B stages, but `SharedStorageDenoise` unions them; on Ada that is the difference between 4 stages and 1 |
| `setup.py` | `COMPUTE_CAPABILITY` is hardcoded to `sm_90a` |

Five kernels use no Hopper constructs and need no porting: `blake3`,
`noise_generation`, `denoise_converter`, `inner_hash`, and `build_routing_data`.

One shared file changed. `TileHashAccumulator::accumulate` in
`../csrc/gemm/pow_utils.hpp` waited on warpgroup MMA before reading the
accumulator, which is right for Hopper and meaningless for Ada's synchronous
atom. Off sm_90a that call is not an error, which is the problem: it becomes
`assert(0)` plus a `printf`, and under `-DNDEBUG` just the `printf` -- executed
once per transcript reduction, in the innermost loop of the miner. It is now
guarded by `CUTE_ARCH_MMA_SM90A_ENABLED`, so the Ada mainloop can reuse the
accumulator rather than keeping a second copy of consensus-critical arithmetic.
The Hopper kernel still builds unchanged (168 registers, no spills).

## Running the checks

`ada/run_checks.sh` builds and runs all of them. The two host checks need only
nvcc; the three device checks are built either way and run when a card is
present, which is worth doing on a machine without one -- register pressure and
shared-memory footprint are compile-time facts, and both are tight on Ada.

    git submodule update --init --depth 1 third_party/cutlass
    ./ada/run_checks.sh

    host checks
    layout_equiv               PASS
    smem_budget                PASS

    device checks
    noisy_gemm                 built, skipped (no GPU)
    noising                    built, skipped (no GPU)
    merkle_roots               built, skipped (no GPU)

    mainloop register pressure (compile-time; Ada allows 255)
      Used 202 registers, 0 bytes spill stores

Each file's own header carries the standalone nvcc line if you want to run one
on its own.

## Files

### `tiled_mma_sm89.hpp`

The Ada MMA tiling, and why it is not a free choice.

### `layout_equiv_sm89.cu`

Proves the Ada tiling is consensus-equivalent to the Hopper one: identical
per-thread accumulator coordinates, row/column patterns matching `settings.py`,
and an unchanged transcript reduction schedule under a smaller `bK`.

cute layouts are compile-time objects, so **this runs on a machine with no GPU**
and belongs in CI:

    nvcc -std=c++20 -x cu -arch=sm_89 -w --expt-relaxed-constexpr \
      -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
      ada/layout_equiv_sm89.cu -o /tmp/layout_equiv && /tmp/layout_equiv

    accumulator layout over a 128x256 tile, 256 MMA threads
      checked 256 threads x 128 registers, 0 coordinate mismatches
      thread 0: 128 registers, rows {0,8}, cols {0,1,8,9,...}

    transcript cadence, K=4096 rank=128
      bK=128 (Hopper): 32 reductions
      bK= 64 (Ada)   : 32 reductions, schedule identical
      bK= 32 (Ada)   : 32 reductions, schedule identical

    PASS: the Ada tiling is consensus-equivalent to the Hopper one

### `smem_budget_sm89.cu`

Computes the shared-memory budget from the production `KernelTraits` layouts and
asserts the proposed Ada regrouping fits in 101376 B. Also no GPU needed, but it
instantiates the Hopper traits to read their layouts, so it compiles for
`sm_90a`:

    nvcc -std=c++20 -x cu -arch=sm_90a -w --expt-relaxed-constexpr \
      -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
      -I csrc ada/smem_budget_sm89.cu -o /tmp/smem_budget && /tmp/smem_budget

### `kernel_traits_sm89.hpp`, `collective_mainloop_sm89.hpp`, `collective_epilogue_sm89.hpp`, `tile_scheduler_sm89.hpp`, `lattice_gemm_sm89.h`

The port itself. `lattice_gemm_sm89.h` carries both the kernel and a launcher
that takes plain pointers, so it can be driven without torch.

### `noising_kernel_sm89.hpp`

Both noising kernels, parameterised by which matrix the rank-R product
multiplies, with a launcher. When the split-K reduction is in use the factor is
accumulated with `atomicAdd`, so its buffer has to be zeroed beforehand -- the
same requirement TMA's reduce-add had.

### `noising_sm89_test.cu`

Correctness against a CPU reference — **needs the GPU**. Covers both roles, the
split-K reduction, and M and K left indivisible by the tile.

    nvcc -std=c++20 -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
      --expt-extended-lambda -DNDEBUG \
      -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
      -I csrc -I ada ada/noising_sm89_test.cu -o /tmp/noising_test && /tmp/noising_test

### `merkle_tree_roots_sm89.hpp`, `merkle_tree_roots_sm89_test.cu`

The Merkle-roots kernel and its test. Only the load path changed, so the test's
reference is a naive kernel with the same structure and no staging at all --
each thread reads its own chunk straight from global memory and hashes it with
the same primitives and the same reduction. Comparing the two isolates what was
ported: whether the staged, swizzled tile presents each thread with the bytes it
would have read itself. **Needs the GPU.**

    nvcc -std=c++20 -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
      --expt-extended-lambda -DNDEBUG \
      -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
      -I csrc -I ada ada/merkle_tree_roots_sm89_test.cu -o /tmp/mt_test && /tmp/mt_test

### `noisy_gemm_sm89_test.cu`

End-to-end correctness against CPU references — **needs the GPU**. Checks the
output tile with the tile exactly covering the problem, with several tiles, and
with M, N and K all leaving remainders, which is what exercises the predication
that replaced TMA's bounds handling.

It also checks the transcript, which the output tile does not cover: the CPU
replays `TileHashAccumulator`'s fold for all 256 threads, the repository's own
BLAKE3 compresses the results, and the PoW target is set to the smallest hash so
exactly one thread can win it. If the kernel's transcripts differ from the
reference by a bit, either the wrong thread wins or none does.

    nvcc -std=c++20 -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
      --expt-extended-lambda -DNDEBUG \
      -I third_party/cutlass/include -I third_party/cutlass/tools/util/include \
      -I csrc -I ada ada/noisy_gemm_sm89_test.cu -o /tmp/ada_test && /tmp/ada_test

### `regpressure_sm89.cu`

Measures the mainloop's register and shared-memory footprint. `ptxas` reports
both at compile time, so this too runs without a GPU — the kernel is only
compiled, never launched:

    nvcc -std=c++20 -x cu -arch=sm_89 -O3 -w --expt-relaxed-constexpr \
      --expt-extended-lambda --use_fast_math -DNDEBUG \
      -DCUTLASS_DEBUG_TRACE_LEVEL=0 \
      --ptxas-options=--verbose,--register-usage-level=10,--warn-on-local-memory-usage \
      -I third_party/cutlass/include -I csrc \
      -c ada/regpressure_sm89.cu -o /dev/null

### `smoke_sm89.cu`

Establishes the premise the whole port depends on: that an SM80-class int8
tensor-core GEMM — the arithmetic the NoisyGEMM mainloop performs — runs
correctly on Ada. It checks results against a CPU reference, because "it ran"
and "it is correct" are different claims. This one does need the GPU.

Verified on a GeForce RTX 4060 Ti (AD106, sm_89), CUDA 12.6.3, CUTLASS v3.9.2:

    int8 GEMM 512x512x256 on sm_89: checked 182 outputs, 0 mismatched
    PASS: SM80 int8 tensor-core path is correct on this GPU

Run it:

    docker run --rm --gpus all -v "$PWD/..":/w -w /w \
      nvidia/cuda:12.6.3-devel-ubuntu24.04 sh -c '
        nvcc -std=c++17 -arch=sm_89 -O2 -w --expt-relaxed-constexpr \
          -I third_party/cutlass/include \
          -I third_party/cutlass/tools/util/include \
          ada/smoke_sm89.cu -o /tmp/smoke && /tmp/smoke'

## Correctness bar

Mining is consensus-critical. A kernel that is subtly wrong does not error — it
produces proofs the verifier rejects, which presents as "mining silently does
not work". Every ported op must be validated against the reference
implementations in `../src/lattice_gemm/testing/` before it is trusted.

# Anti-ASIC posture

## What Lattice claims, and what it does not

Lattice does **not** claim to be ASIC-resistant, and no part of the protocol
tries to cryptographically prevent specialised mining hardware. That is not
reliably achievable — the history of "ASIC-resistant" algorithms is largely a
history of ASICs arriving anyway, a year or two later, while the claim was still
on the website. Pearl does not make the claim either, and inheriting it would be
dishonest.

There is also a specific reason Lattice *shouldn't* fight specialised hardware
too hard: the work function is integer matrix multiplication, deliberately
shaped so that ordinary machine-learning accelerators are useful for mining.
That is the entire premise of proof-of-*useful*-work. Hardware that is good at
int8 matmul is hardware that is good at the thing the network is nominally
buying. A GPU is already a matmul ASIC.

The concern is narrower than "specialised hardware exists". It is
**concentration**: a single party gaining a large hashrate advantage from
hardware nobody else can obtain.

## What Lattice does instead

A social process, backed by a code structure that makes the process cheap to
carry out.

The work function's shape parameters — tile geometry, operand precision, the
minimum noise rank — are collected in one clearly labelled, version-tagged
module: [`zk-pow/src/work_function_params.rs`](../zk-pow/src/work_function_params.rs).

That module does not introduce a runtime configuration point. Nothing reads
those values from a config file, a flag, an environment variable, or chain
state. They are compile-time constants that must match the proving circuit
exactly, and static assertions fail the build if they ever drift apart from the
circuit's own definitions.

Its purpose is **auditability of change**. If the parameters ever need retuning,
the diff is small, centralised, and readable in one sitting — rather than an
archaeology expedition across the circuit, the prover, the verifier, and the
FFI. A hardfork proposal that nobody can review is a hardfork proposal that
nobody should accept.

## When retuning would be considered

Roughly: evidence that mining power has concentrated in hardware that is not
generally available. Signals worth watching include a hashrate increase far
outpacing the rate at which commodity accelerators could plausibly have been
deployed, sustained hashrate that does not respond to price the way a
GPU-dominated market would, or block production concentrating into a small
number of addresses over an extended period.

None of these is conclusive alone. Hashrate spikes for boring reasons too.

## The process

This stage of the project is small, so the process is correspondingly informal —
described honestly rather than dressed up as governance it does not yet have.

1. **Publish the evidence.** A maintainer opens a public proposal documenting
   what was observed and why it suggests hardware concentration, including the
   data behind it.
2. **Propose specific parameters.** Name the new values, the new
   `WORK_FUNCTION_VERSION`, and an activation height far enough out that
   everyone can upgrade in time.
3. **Discuss in the open.** Miners, node operators, and anyone else. Expect
   disagreement: retuning deliberately devalues existing hardware, including
   honest miners' hardware. That is a real cost and it belongs in the debate.
4. **Adopt by upgrading.** Nodes and miners run the new build, or they do not.
   There is no key that forces it and no vote that binds anyone. A fork that
   nobody adopts simply does not happen.

That last point is the substantive guarantee. There is no admin capability here.
The maintainers' power is limited to publishing a proposal and arguing for it.

## Retuning checklist

Any change to these parameters is a consensus break requiring a coordinated
hardfork:

1. Bump `WORK_FUNCTION_VERSION`.
2. Update the constants in `work_function_params.rs`. The static assertions
   will fail the build until the circuit agrees.
3. Regenerate the proving/verifying key caches (`task build:zk-cache`).
4. Add an activation height in `node/chaincfg/params.go`, following the existing
   fork-height pattern (`MoEForkHeight`, `SaltedSeedForkHeight`, and friends).
5. Publish the proposal per the process above.

## Current parameters

| Parameter | Value | Retuning effect |
|---|---|---|
| `WORK_FUNCTION_VERSION` | 1 | Identifies the parameter generation. |
| `TILE_HEIGHT` | 2 | Widens the systolic shape a competitive miner must build. |
| `TILE_DEPTH` | 16 | Deepens each accumulation; shifts the compute-to-memory ratio. |
| `OPERAND_PRECISION_BITS` | 8 | Heaviest retune available; affects trace layout throughout the circuit. |
| `MIN_NOISE_RANK` | 128 | Raises the cost floor of any low-rank shortcut. |
| `JACKPOT_WORDS` | 16 | Gates block acceptance. |
| `ROTATION_PER_TILE` | 13 | Diffusion; retuned alongside tile geometry. |

Of these, tile geometry is the realistic lever. Changing operand precision would
be close to redesigning the work function, and would undermine the useful-work
premise if it moved away from formats real accelerators implement.

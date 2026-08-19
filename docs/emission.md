# Lattice emission: the sawtooth reward schedule

This is the one thing about Lattice that differs meaningfully from Pearl, the
chain it forks. Everything else — the proof-of-useful-work consensus, the ZK
verification, the matmul work function — is inherited unchanged. So it is worth
explaining properly.

## The short version

- Every block pays a reward, starting at **exactly 2 LATT**.
- The reward **decays smoothly** as the chain grows — no halvings, no cliffs.
- The decay is **gentle**: still 1.92 LATT after a year, 1.71 LATT after four.
- After about four years, the schedule **resets** and the reward jumps back to
  2 LATT. Then it starts over. Forever.
- There is **no maximum supply** and **no point at which rewards stop**.
- There was **no premine**. The genesis block created zero coins.

## Why a reset at all?

A pure decay curve converges. Given long enough, the reward approaches zero and
mining stops paying for the security it provides. Bitcoin handles this by
assuming transaction fees will eventually replace the subsidy. That may work.
It is also an untested bet on an economy that does not exist yet.

Lattice does not make that bet. Instead the emission curve restarts on a fixed,
published schedule. Miners are paid roughly the same amount in year twenty as in
year one, so the security budget does not quietly evaporate. The cost is that
supply grows without bound, which is a real tradeoff and stated plainly here
rather than buried: **Lattice is mildly inflationary by design, permanently.**

What it is not is *unpredictably* inflationary. Every future reward is fixed at
launch and computable by anyone.

## The curve

Within one epoch, the reward for the i-th block of that epoch is

```
reward(i) = S·k / ((i+k)·(i−1+k))          [in cells]
```

Summed from 1 to i this telescopes to a closed form:

```
cumulative(i) = S·i / (i+k)
```

which converges toward `S` without ever reaching it. Two constants shape it:

| Constant | Symbol | Mainnet value | Meaning |
|---|---|---|---|
| Emission constant | `k` | 39,420,000 blocks | Sets how gradual the decay is. Equal to 50 years of blocks at the 40s target. |
| Reference supply | `S` | 78,840,002 LATT | The value one epoch's emission converges toward. A shape parameter, **not** a supply cap. |

`S` and `k` are chosen so the first block of every epoch pays exactly
`S/(1+k) = 2 LATT`.

Because `k` (50 years of blocks) is much larger than an epoch (4 years), the
chain only ever rides the gentle opening stretch of the curve. That is what
keeps the reward near 2 LATT instead of collapsing toward zero.

## The reset

| Constant | Symbol | Mainnet value |
|---|---|---|
| Reset threshold | `T` | 5,840,000 LATT |
| Epoch length | `L` | 3,153,600 blocks (≈ 4.00 years) |

The rule is stated economically: **when cumulative emission since the last reset
reaches `T`, the schedule resets.** Consensus evaluates it mechanically: **the
schedule resets every `L` blocks.**

These are the same rule. `L` is not an independent knob — it is precisely the
smallest number of blocks whose (integer-truncated) rewards sum to `T`. A test,
`TestEpochBlocksMatchesThreshold`, recomputes this from scratch on every build
and fails if the two ever disagree. Consensus uses `L` because it is O(1) and
cannot drift; the threshold is what the design *means*.

### Reward over one epoch

| Point in epoch | Height in epoch | Reward |
|---|---|---|
| First block | 1 | 2.000000 LATT |
| After 1 day | 2,160 | 1.999781 LATT |
| After 1 month | 65,700 | 1.993350 LATT |
| After 1 year | 788,400 | 1.922338 LATT |
| After 2 years | 1,576,800 | 1.849113 LATT |
| Last block | 3,153,600 | 1.714678 LATT |
| **Next block (reset)** | **1** | **2.000000 LATT** |

One epoch mints 5,840,000.13 LATT. Every epoch mints exactly the same amount, so
total issued supply after *n* complete epochs is simply *n* × that figure.

## Predictability guarantees

These are the properties the design exists to provide.

**Every future reset height is computable now, offline, by anyone.** Reset
heights are `n·L + 1` for n = 1, 2, 3, … The first is block 3,153,601, the
second 6,307,201, and so on indefinitely. No chain state is needed beyond the
current height.

**Nothing can change the schedule.** `k`, `S`, `T`, and `L` are compile-time
constants in `node/chaincfg/emission.go`. There is no governance mechanism, no
admin key, no creator privilege, and no configuration file that alters them.
Changing them requires shipping a different binary that every node operator must
consciously choose to run — in other words, a hardfork.

**Resets never touch existing coins.** A reset changes only the reward of
*future* blocks. Balances, historical blocks, and already-mined supply are
untouched. There is no rebasing, no dilution event, no supply adjustment. The
reward for any past height is a pure function of that height and returns the same
answer forever.

**Every node computes it identically.** The reward depends on block height and
hardcoded constants — nothing else. Not on time, peers, difficulty, or fees.

## Checking it yourself

Ask a running node where it is in the schedule:

```bash
latctl getnextreset
```

```json
{
  "height": 205,
  "epoch": 1,
  "epochblocks": 200,
  "heightinepoch": 5,
  "epochstartheight": 201,
  "nextresetheight": 401,
  "blocksuntilreset": 196,
  "estimatedsecondsuntilreset": 7840,
  "blockssincelastreset": 5,
  "currentreward": 1.9936166,
  "nextreward": 1.99202552,
  "initialreward": 2,
  "mintedthisepoch": 9.98403192,
  "resetthreshold": 370.51851754,
  "totalsupply": 380.50254946,
  "targetblockseconds": 40
}
```

(Values above are from simnet, whose epoch is deliberately tiny.)

`nextresetheight` and `blocksuntilreset` are **exact**. The two `estimated`
fields are genuinely estimates: they assume blocks keep arriving at the 40 second
target, and real block times vary with hashrate and difficulty retargeting.

## Per-network parameters

Test networks compress the identical curve shape into a short epoch so a reset
can be observed in minutes rather than years. The ratio `L/k` is held at 0.08 on
every network, so the reward decays through exactly the same shape everywhere —
every network starts an epoch at 2.000000 LATT and ends it near 1.715 LATT.

| Network | `k` | `S` (LATT) | `T` (LATT) | `L` (blocks) | Epoch duration |
|---|---|---|---|---|---|
| mainnet | 39,420,000 | 78,840,002 | 5,840,000 | 3,153,600 | ≈ 4 years |
| testnet / testnet2 / signet | 125,000 | 250,002 | 18,518.67 | 10,000 | ≈ 4.6 days |
| simnet / regtest | 2,500 | 5,002 | 370.52 | 200 | ≈ 2.2 hours |

## Why these numbers

The starting reward of 2 LATT and the roughly four-year epoch are deliberate
choices; the rest follows from them.

Block time is 40 seconds, so a solo miner with a modest share of network hashrate
still lands blocks regularly rather than waiting months for one large payout.
Frequent small rewards are what make solo mining viable without a pool, which is
the point — a chain that forces everyone into pools re-centralises the thing
proof-of-work is supposed to decentralise.

Given 2 LATT blocks every 40 seconds, annual issuance is roughly 1.5 million LATT
in the first year and declines slowly within each epoch. As a percentage of
circulating supply this inflation rate falls over time, since the denominator
grows while per-epoch issuance stays flat.

## Where the code lives

| File | Contents |
|---|---|
| `node/chaincfg/emission.go` | The constants, per network, with the design rationale. |
| `node/blockchain/emission.go` | The curve, the epoch arithmetic, and the reset. |
| `node/blockchain/emission_test.go` | Threshold/epoch-length invariant, exactness checks. |
| `node/blockchain/emission_chain_test.go` | Multi-epoch sawtooth behaviour. |
| `node/rpcserver.go` | `handleGetNextReset`. |

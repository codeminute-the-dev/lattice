# Solo mining Lattice

## Why solo, and not a pool

Mining rewards are lottery tickets. What makes people join pools is *variance* —
if blocks are rare and rewards large, a small miner can go months without income
even while earning a fair long-run average. Pools smooth that out, at the cost of
concentrating block production in a handful of operators.

Lattice attacks the variance directly instead: **40-second blocks** and **2 LATT
per block**, rather than rare large payouts. There are about 2,160 blocks a day.
A miner holding 1% of network hashrate expects roughly 21 blocks a day — income
that arrives steadily rather than as an annual windfall.

Concretely, at various shares of network hashrate:

| Your share | Expected blocks/day | Expected LATT/day | Typical wait between blocks |
|---|---|---|---|
| 10%   | ~216   | ~432   | ~7 minutes |
| 1%    | ~21.6  | ~43    | ~1.1 hours |
| 0.1%  | ~2.2   | ~4.3   | ~11 hours |
| 0.01% | ~0.22  | ~0.43  | ~4.6 days |

Down to roughly 0.1% of hashrate, solo mining pays out often enough that pooling
buys very little. This is a deliberate design choice, not an accident of
parameters.

None of this removes the underlying randomness — block finding is still a Poisson
process, and short-run luck varies. It just makes the runs short.

## Before you start

This is alpha software with no public network. Everything below runs against
your own local chain. LATT has no value; treat this as dogfooding.

## 1. Build

See the [README](../README.md#building). You need `bin/latticed`, `bin/latctl`,
`bin/latwallet`, and `bin/latwalletcli`, built with `-tags xmss,zkpow`.

## 2. Create a wallet and get an address

The interactive path walks you through wallet creation and address generation:

```bash
cd bin && ./latwalletcli
```

Or manually:

```bash
./bin/latwallet -u rpcuser -P rpcpass --create      # follow prompts, record your seed
./bin/latwallet -u rpcuser -P rpcpass &
./bin/latctl -u rpcuser -P rpcpass -s https://localhost:44207 getnewaddress
```

Write the seed down offline. It is the only way to recover the wallet.

## 3. Start a node on simnet

Simnet is a private network with a 200-block emission epoch, so you can watch a
full reset in a couple of hours of mining — or in seconds with `generate`.

```bash
./bin/latticed --simnet \
  --rpcuser=rpcuser --rpcpass=rpcpass \
  --miningaddr=<your-rlat1-address> \
  --txindex
```

For testnet, swap `--simnet` for `--testnet` (10,000-block epochs, ~4.6 days).

Useful flags: `--notls` to drop TLS on a local socket, `--debuglevel=debug` for
verbose logs. See `node/sample-latticed.conf` for the full set.

## 4. Mine

On simnet you can mine blocks directly:

```bash
./bin/latctl --simnet -u rpcuser -P rpcpass generate 10
```

On testnet or mainnet, mining runs through the GPU miner. It has two parts:
`lattice-gateway` (bridges to the node) and `vllm-miner` (does the matmul work).

```bash
export LATTICED_RPC_URL="http://localhost:44109"
export LATTICED_RPC_USER="rpcuser"
export LATTICED_RPC_PASSWORD="rpcpass"
export LATTICED_MINING_ADDRESS="<your-address>"
lattice-gateway start
```

The gateway exposes a mining interface on `/tmp/latticegw.sock`, or port 8337
with `MINER_RPC_TRANSPORT=tcp`.

## 5. Watch the emission schedule

This is the part worth checking, since it is what makes Lattice different:

```bash
./bin/latctl --simnet -u rpcuser -P rpcpass getnextreset
```

```json
{
  "height": 205,
  "epoch": 1,
  "epochstartheight": 201,
  "nextresetheight": 401,
  "blocksuntilreset": 196,
  "estimatedsecondsuntilreset": 7840,
  "currentreward": 1.9936166,
  "initialreward": 2,
  "mintedthisepoch": 9.98403192,
  "totalsupply": 380.50254946
}
```

`nextresetheight` and `blocksuntilreset` are exact — they are arithmetic on
hardcoded constants, not forecasts. The `estimated` time fields assume blocks
keep arriving at the 40-second target and will drift with real hashrate.

Mine past a reset boundary on simnet and watch the reward jump back to 2 LATT:

```bash
./bin/latctl --simnet -u rpcuser -P rpcpass generate 200
# reward at height 200: 1.71599906 LATT
# reward at height 201: 2.00000000 LATT   <- reset
```

## 6. Check your balance

```bash
./bin/latctl -u rpcuser -P rpcpass -s https://localhost:44207 getbalance
```

Coinbase rewards need 100 confirmations before they can be spent — about
67 minutes at 40-second blocks.

## Troubleshooting

**`zkpow: build with -tags zkpow to enable mining`** — the node was built without
the proving library. Rebuild with `-tags xmss,zkpow` after building the Rust
crates.

**Blocks are not being found on testnet** — check the node has peers
(`latctl getpeerinfo`) and is synced (`latctl getblockchaininfo`). There is no
public network yet, so expect zero peers unless you are running your own.

**`not enough funds for coin selection`** — coinbase outputs are still maturing.
Wait 100 blocks.

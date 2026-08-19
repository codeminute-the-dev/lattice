# Lattice-Domain Hard Fork Upgrade Guide for Miners and Mining Pools

Lattice has one hard fork scheduled. At the **fork height**, blocks switch from the V3
certificate to the V4 certificate. The two are byte-identical on the wire; what changes
is the pair of constants used to salt the matrix commitments before the noise-seed
chain.

| Network  | Fork height (`LatticeSeedForkHeight`) |
| -------- | ------------------------------------- |
| Mainnet  | `394200`                              |
| Testnet  | `43200`                               |
| Testnet2 | `43200`                               |
| Regtest  | disabled                              |
| Simnet   | disabled                              |

## Why this fork exists

Lattice's V3 proof of work is Pearl's, byte for byte, on purpose.

The reference miner (`lattice-gemm`) requires H100-class hardware, and no independent
Lattice miner has been written. Sharing Pearl's proof-of-work domain is what lets an
existing third-party miner for Pearl's algorithm produce valid Lattice blocks without
knowing Lattice exists — the pool speaks Pearl's algorithm to the miner and Lattice to
the chain. That is the only consumer-GPU mining this chain has on day one.

The cost is that the two chains draw from one pool of work, and the smaller chain is the
one a bored Pearl miner can reorg. So the sharing is scheduled, not permanent: the fork
height is far enough out to give a native Lattice miner time to appear — mining demand
being the thing that makes someone write one — and near enough that the chain does not
sit in a borrowed domain indefinitely.

Be clear about what the split does and does not buy. A salt is a constant, not a
barrier: anyone willing to recompile a miner crosses it in an afternoon. It deters the
incidental cross-mining that costs an attacker nothing, not a determined one.

## What changes

Only the two salt constants:

```text
V3:  key_a = blake3("pearl/cert-v3/noise-seed/A")
     key_b = blake3("pearl/cert-v3/noise-seed/B")

V4:  key_a = blake3("lattice/cert-v4/noise-seed/A")
     key_b = blake3("lattice/cert-v4/noise-seed/B")
```

Everything else — the bind message, the seed chain, the MoE routing fold, the circuits,
the wire layout, the share format — is exactly as documented in
[salted-seed-fork-upgrade-guide.md](salted-seed-fork-upgrade-guide.md).

The V4 wire layout is identical to V3 (`Version(4) | HeaderHash(32) | PublicDataLen(4) |
PublicData(N) | ProofDataLen(4) | ProofData`) with `4` in the version field, and the
header's proof commitment is `double_sha256(cert_version_le32 + public_data)` with the
prefix now `4`.

A share is bound to one derivation. A share mined under V3 rules fails V4 verification
and vice versa, so verify shares with the version of the template they were mined
against.

## Step 1: Node

`getblocktemplate` reports `requiredcertversion: 4` at and after the fork height (`3`
before it). Read the version from the template; do not hardcode the fork height.

## Step 2: ZK proving code (pools)

If you use the certificate-version dispatchers, no code change is needed beyond the
package upgrade — `check_cert_version_eligible`,
`generate_proof_for_cert_version`, `verify_proof_for_cert_version`, and
`verify_plain_proof_for_cert_version` accept version 4. Older package versions raise
`ValueError: unknown certificate version: 4`.

Versioned entry points exist too: `generate_proof_v4`, `verify_proof_v4`,
`verify_plain_proof_v4` — same circuits and circuit cache as V3; only the salts differ.

If you call the C FFI directly, `verify_zk_proof_v4` and `verify_zk_proof_v4_with_nbits`
are the V4 counterparts of the `_v3` functions; `mine` and the rest already take a
`cert_version` argument and accept `4`. Rebuild against the new header.

If you use the Rust crate directly, `SeedDerivation::SaltedLattice` is the V4 variant,
and `CertificateVersion::ZkV4.seed_derivation()` maps to it. Reference:
`zk-pow/src/api/seed.rs`, which carries the hardcoded salts and pinned test vectors for
both domains.

## Step 3: Miners

Required. From the fork height on, a miner still salting with the V3 constants produces
shares the pool will reject.

If the job's `cert_version` is 4, use the V4 salts in the bind step:

```text
bound_a = blake3(hash_a || m_le32 || 0^28, key = blake3("lattice/cert-v4/noise-seed/A"))
bound_b = blake3(hash_b || n_le32 || 0^28, key = blake3("lattice/cert-v4/noise-seed/B"))
```

Nothing else about the derivation changes.

Reference implementations: `zk-pow/src/api/seed.rs` (Rust, with pinned vectors for both
domains) and `miner/miner-base/src/miner_base/commitment_hash.py` (Python,
`SEED_SALT_A`/`SEED_SALT_B` for V3 and `LATTICE_SEED_SALT_A`/`LATTICE_SEED_SALT_B` for
V4).

The `lattice-gemm` CUDA kernel implements V3 only. It requires sm90 hardware and is not
the miner this chain expects to be running by the fork height; if you depend on it,
extend `commitment_hash_from_merkle_roots_kernel.hpp` to select the salt pair from the
job's certificate version.

## Questions

Contact the Lattice team on the usual channels if anything is unclear.

# The V3 (Salted-Seed) Certificate

> **Lattice has no V2→V3 migration.** Every Lattice network activates V3 at height 1
> (`SaltedSeedForkHeight: 1`), so there is no era of this chain in which V2 blocks were
> ever valid. This document is the V3 derivation reference, kept because pools and
> custom miners need the spec. The fork Lattice *does* have scheduled is the V4 cutover
> — see [lattice-seed-fork-upgrade-guide.md](lattice-seed-fork-upgrade-guide.md).

V3 blocks carry a ZK certificate whose noise seeds are derived from *salted* matrix
commitments rather than the raw Merkle roots the V2 (MoE) certificate used.

**The short version:**

- V3 changes how the noise seeds are derived from the matrix commitments: each Merkle
  root is first salted with a keyed BLAKE3 hash that also commits the matrix dimensions
  (`m` for A, `n` for B). The ZK circuits, wire formats, and share formats are unchanged.
- Because the derivation feeds mining itself, **node, ZK proving code, and miners must
  all agree on it.** A miner deriving seeds the V2 way produces shares Lattice rejects.
- The certificate version comes from `getblocktemplate` per block. Read it from the
  template; never hardcode a version or a fork height.

**The salts are Pearl's, deliberately.** Lattice's V3 work function is bit-identical to
Pearl's, so a third-party miner implementing Pearl's algorithm produces valid Lattice
work without knowing Lattice exists. The reference miner needs H100-class hardware and
no independent Lattice miner has been written yet, so that shared domain is the only
consumer-GPU mining this chain has. It is scheduled to end at `LatticeSeedForkHeight`.

---

## Node support

`getblocktemplate` reports `requiredcertversion: 3` from height 1 (and `4` from
`LatticeSeedForkHeight` on).

## ZK proving code (pools)

If you use the certificate-version dispatchers, the `lattice-mining` package handles
version 3:

- `check_cert_version_eligible`, `generate_proof_for_cert_version`,
  `verify_proof_for_cert_version`, and `verify_plain_proof_for_cert_version` handle
  `requiredcertversion` 1, 2, and 3. Old package versions raise
  `ValueError: unknown certificate version: 3`.
- Versioned entry points exist too: `generate_proof_v3`, `verify_proof_v3`,
  `verify_plain_proof_v3` (same circuits and circuit cache as V2; only the seed
  derivation differs).
- A share is bound to one derivation. A share mined under V2 rules fails V3
  verification and vice versa, so verify shares with the version of the template
  they were mined against.

If you call the C FFI directly: `mine`, `mine_moe`, `verify_plain_proof_ffi`, and
`prove_plain_proof_ffi` now take a `cert_version` argument. Rebuild against the new
header; do not run old binaries against the new library.

If you use the Rust crate directly: pass `SeedDerivation` (from
`zk_pow::api::proof`) to `zk_prove_plain_proof` / `verify_plain_proof`, or map it
from the certificate version with `CertificateVersion::seed_derivation()`.
Reference: `zk-pow/src/api/seed.rs`.

Only if you serialize certificates yourself: the V3 wire layout is identical to V2
(`Version(4) | HeaderHash(32) | PublicDataLen(4) | PublicData(N) | ProofDataLen(4) |
ProofData`) with `3` in the version field, and the header's proof commitment is
`double_sha256(cert_version_le32 + public_data)` with the prefix now `3`.

## Miners

V3 share noise uses salted seeds, so mining software that derives seeds the V2 way
produces invalid shares. Pools should expect
`verify_plain_proof_for_cert_version(3, ...)` to reject them.

The mining job carries `cert_version` (a required field of `submitPlainProof`); the
miner reads it per job and salts when the job requires V3.

If you built custom mining software, implement the derivation:

1. Compute the keyed Merkle roots of A and B exactly as today (`hash_a`, `hash_b`).
   The wire formats do not change; shares still carry the raw roots.
2. When the job's `cert_version` is 3, bind each root to its matrix dimension
   before the (unchanged) seed chain:

   ```text
   bound_a = blake3(hash_a || m_le32 || 0^28, key = blake3("pearl/cert-v3/noise-seed/A"))
   bound_b = blake3(hash_b || n_le32 || 0^28, key = blake3("pearl/cert-v3/noise-seed/B"))
   ```

   Each message is exactly one 64-byte BLAKE3 block: the 32-byte root, the
   dimension as a little-endian u32, and 28 zero bytes.
3. Use the bound roots wherever the raw roots fed the seed chain:
   `b_noise_seed = blake3(job_key || bound_b)`, then
   `a_noise_seed = blake3(b_noise_seed || bound_a)`.
4. MoE only: salt **before** the routing fold — `bound_a` (not `hash_a`) goes into
   `hash_activations = blake3(bound_a || hash_routing)`. The dimensions are
   `m` = token count and `n` = the **per-expert** intermediate dimension (B holds
   all experts stacked, but `n` does not include the expert count).

Reference implementations: `zk-pow/src/api/seed.rs` (Rust, with pinned test
vectors), `miner/miner-base/src/miner_base/commitment_hash.py` (Python,
`bind_root_a`/`bind_root_b`), and the CUDA kernel behind
`commitment_hash_from_merkle_roots(..., salted_dims=(m, n))` in `lattice-gemm`.

## Questions

Contact the Lattice team on the usual channels if anything is unclear.

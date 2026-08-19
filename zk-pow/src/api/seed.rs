//! Salted (certificate V3/V4) Merkle-root pre-hashing and the seed-derivation
//! selector. The seed chain lives in `PublicProofParams::commitment_hash`.

use crate::api::proof::Hash256;
use lattice_blake3::blake3_digest;

/// How the noise seeds are derived from the Merkle roots. Selected by the
/// certificate version; never serialized (the wire always carries raw roots).
/// No `Default`: a silent default would verify under the wrong rules.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SeedDerivation {
    /// Pre-V3 chain: `b_noise_seed = blake3(job_key || hash_b)`, etc.
    Legacy,
    /// V3 chain: roots are salted via [`bind_root_a`]/[`bind_root_b`] before
    /// the (unchanged) seed chain, under the inherited Pearl salts.
    Salted,
    /// V4 chain: identical construction to [`Self::Salted`], under Lattice's
    /// own salts. See [`LATTICE_SEED_SALT_A`] for why the split is deferred to
    /// a fork height rather than applied at launch.
    SaltedLattice,
}

impl SeedDerivation {
    pub fn bind_roots(self, hash_a: &Hash256, hash_b: &Hash256, m: u32, n: u32) -> (Hash256, Hash256) {
        match self {
            Self::Legacy => (*hash_a, *hash_b),
            Self::Salted => (bind_root_a(hash_a, m), bind_root_b(hash_b, n)),
            Self::SaltedLattice => (bind_root_lattice_a(hash_a, m), bind_root_lattice_b(hash_b, n)),
        }
    }
}

/// Domain-separation salt for A's root: `blake3("pearl/cert-v3/noise-seed/A")`.
/// Hardcoded so consensus doesn't depend on runtime string hashing (re-derived in a test).
///
/// These are Pearl's salts, deliberately. Lattice inherits Pearl's V3 work
/// function byte for byte, so an existing third-party `pearlhash` miner does
/// valid Lattice work without knowing Lattice exists. That is the only way a
/// new chain gets consumer-GPU mining on day one: the reference miner needs
/// H100-class hardware, and no independent Lattice miner has been written yet.
/// [`LATTICE_SEED_SALT_A`] is where the chain moves once one has.
pub const SEED_SALT_A: Hash256 = [
    0x82, 0x49, 0x40, 0x6c, 0xa0, 0xed, 0x15, 0x16, 0x96, 0x16, 0xf6, 0x92, 0xfc, 0xf0, 0x76, 0xf8, 0x92, 0xdb, 0xdb, 0x2a, 0x70,
    0x23, 0xb8, 0x52, 0xf0, 0xd4, 0x77, 0x19, 0xc3, 0x90, 0x01, 0x7b,
];

/// Domain-separation salt for B's root: `blake3("pearl/cert-v3/noise-seed/B")`.
pub const SEED_SALT_B: Hash256 = [
    0x11, 0x30, 0x06, 0x32, 0xec, 0x63, 0x01, 0xca, 0x2b, 0xe2, 0xaf, 0x71, 0x8b, 0x3f, 0x4d, 0x4f, 0x1a, 0xe9, 0xc6, 0x39, 0x88,
    0xe8, 0xcc, 0x04, 0x48, 0x44, 0x30, 0x1d, 0x71, 0xb8, 0x9a, 0xa9,
];

/// V4 domain-separation salt for A's root: `blake3("lattice/cert-v4/noise-seed/A")`.
///
/// Sharing [`SEED_SALT_A`]'s domain with Pearl means the two chains draw from
/// one pool of work, and the smaller chain is the one a bored Pearl miner can
/// reorg. Lattice accepts that exposure only while it has no miner of its own;
/// `LatticeSeedForkHeight` in `node/chaincfg/params.go` is the height at which
/// these salts take over and the chains stop sharing a PoW domain.
///
/// Note what separation does and does not buy. A salt is a constant, not a
/// barrier: anyone willing to recompile a miner crosses it in an afternoon. It
/// deters the incidental cross-mining that costs an attacker nothing, not a
/// determined one.
pub const LATTICE_SEED_SALT_A: Hash256 = [
    0x14, 0x23, 0xe8, 0x9e, 0xba, 0x72, 0x10, 0x73, 0xcc, 0xd9, 0x2a, 0x7e, 0xfb, 0xc3, 0xc7, 0xbe, 0x93, 0x6a, 0x0a, 0x1e, 0x10,
    0xd3, 0xe8, 0x9c, 0x8d, 0x97, 0x05, 0x25, 0x33, 0xe8, 0x0e, 0x5b,
];

/// V4 domain-separation salt for B's root: `blake3("lattice/cert-v4/noise-seed/B")`.
pub const LATTICE_SEED_SALT_B: Hash256 = [
    0x4e, 0xf8, 0x95, 0xab, 0xa3, 0x25, 0x8b, 0x1a, 0x18, 0x39, 0x96, 0x14, 0x43, 0xb7, 0x68, 0xb8, 0x8a, 0x35, 0x9d, 0x7e, 0xb8,
    0x06, 0x28, 0x86, 0x77, 0x07, 0x41, 0xa4, 0x40, 0xdf, 0x82, 0x66,
];

/// `root || dim(u32 LE) || 28 zero bytes` — a single 64-byte BLAKE3 block.
fn bind_message(root: &Hash256, dim: u32) -> [u8; 64] {
    let mut msg = [0u8; 64];
    msg[..32].copy_from_slice(root);
    msg[32..36].copy_from_slice(&dim.to_le_bytes());
    msg
}

/// V3 salting of A's Merkle root: `blake3(hash_a || pad32(m), key=SEED_SALT_A)`.
/// Commits the row count `m`.
pub fn bind_root_a(hash_a: &Hash256, m: u32) -> Hash256 {
    blake3_digest(&bind_message(hash_a, m), Some(SEED_SALT_A))
}

/// V3 salting of B's Merkle root: `blake3(hash_b || pad32(n), key=SEED_SALT_B)`.
/// Commits the column count `n` (the per-expert `n_e` in MoE; `e` rides on the job_key).
pub fn bind_root_b(hash_b: &Hash256, n: u32) -> Hash256 {
    blake3_digest(&bind_message(hash_b, n), Some(SEED_SALT_B))
}

/// V4 salting of A's Merkle root: [`bind_root_a`] under [`LATTICE_SEED_SALT_A`].
pub fn bind_root_lattice_a(hash_a: &Hash256, m: u32) -> Hash256 {
    blake3_digest(&bind_message(hash_a, m), Some(LATTICE_SEED_SALT_A))
}

/// V4 salting of B's Merkle root: [`bind_root_b`] under [`LATTICE_SEED_SALT_B`].
pub fn bind_root_lattice_b(hash_b: &Hash256, n: u32) -> Hash256 {
    blake3_digest(&bind_message(hash_b, n), Some(LATTICE_SEED_SALT_B))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::api::proof::{MoEParams, PublicProofParams};

    #[test]
    fn seed_salts_match_context_strings() {
        assert_eq!(SEED_SALT_A, blake3_digest(b"pearl/cert-v3/noise-seed/A", None));
        assert_eq!(SEED_SALT_B, blake3_digest(b"pearl/cert-v3/noise-seed/B", None));
        assert_eq!(LATTICE_SEED_SALT_A, blake3_digest(b"lattice/cert-v4/noise-seed/A", None));
        assert_eq!(LATTICE_SEED_SALT_B, blake3_digest(b"lattice/cert-v4/noise-seed/B", None));
    }

    /// The V3 domain must stay bit-identical to Pearl's, or third-party
    /// `pearlhash` miners silently produce work this chain rejects.
    #[test]
    fn v3_and_v4_domains_are_distinct() {
        assert_ne!(SEED_SALT_A, LATTICE_SEED_SALT_A);
        assert_ne!(SEED_SALT_B, LATTICE_SEED_SALT_B);
        let (a3, b3) = SeedDerivation::Salted.bind_roots(&[0xAA; 32], &[0xBB; 32], 192, 320);
        let (a4, b4) = SeedDerivation::SaltedLattice.bind_roots(&[0xAA; 32], &[0xBB; 32], 192, 320);
        assert_ne!(a3, a4);
        assert_ne!(b3, b4);
    }

    /// Pinned vectors (independently computed with Python `blake3`).
    #[test]
    fn commitment_hash_pinned_vectors() {
        let job_key = [0x11u8; 32];
        let cases = [
            (
                SeedDerivation::Legacy,
                [
                    0xad, 0xd6, 0xf7, 0xea, 0x5f, 0xee, 0xbf, 0x89, 0xc8, 0xa7, 0x7e, 0x2e, 0xbf, 0xa0, 0xd8, 0x24, 0x42, 0xe7,
                    0xdb, 0xb0, 0x04, 0x6d, 0xbd, 0x48, 0x97, 0x18, 0x61, 0xd1, 0x2f, 0xcb, 0x01, 0x77,
                ],
                [
                    0x48, 0x3b, 0x07, 0xb6, 0xf7, 0x31, 0x05, 0x03, 0x0b, 0x94, 0x82, 0x25, 0x5f, 0x37, 0x72, 0x3f, 0x3f, 0xed,
                    0x69, 0xae, 0x91, 0x67, 0x24, 0xee, 0x82, 0x91, 0x84, 0x8b, 0x8c, 0x28, 0x79, 0x4b,
                ],
            ),
            (
                SeedDerivation::Salted,
                [
                    0x60, 0xed, 0x9b, 0x73, 0xc5, 0xa9, 0x59, 0x9b, 0x20, 0x0b, 0x6c, 0xd5, 0x63, 0xe7, 0xf0, 0xd5, 0xd9, 0xa6,
                    0x7d, 0x24, 0x02, 0xd8, 0x5f, 0xd4, 0xef, 0x96, 0x6c, 0x58, 0x00, 0x80, 0xd0, 0xe5,
                ],
                [
                    0x30, 0x17, 0x84, 0x16, 0x80, 0x05, 0xec, 0x83, 0x3a, 0xb0, 0xaa, 0x60, 0x00, 0x6f, 0x7f, 0xe7, 0xfa, 0xaa,
                    0x95, 0x30, 0x7d, 0x8c, 0x1f, 0xc6, 0x81, 0x9b, 0x2f, 0xfd, 0xd7, 0x17, 0xec, 0xcf,
                ],
            ),
            (
                SeedDerivation::SaltedLattice,
                [
                    0x32, 0x84, 0xfc, 0xae, 0x62, 0x3a, 0x9b, 0xca, 0xfb, 0xb0, 0xae, 0x26, 0xcc, 0x41, 0xad, 0xca, 0x21, 0x3a,
                    0x62, 0x4f, 0xb5, 0x9a, 0xf3, 0xf6, 0xce, 0x22, 0xf9, 0x2d, 0x5c, 0x70, 0x4a, 0x5e,
                ],
                [
                    0xbc, 0x61, 0x39, 0x6b, 0x8b, 0x3e, 0xb1, 0x09, 0x5d, 0x34, 0x87, 0x09, 0xd0, 0xc9, 0xe1, 0xd3, 0x45, 0x94,
                    0xc1, 0x41, 0x82, 0xbd, 0xdd, 0x29, 0x95, 0x3a, 0xac, 0x4a, 0x23, 0xca, 0x68, 0x18,
                ],
            ),
        ];
        for (derivation, expected_b, expected_a) in cases {
            let mut p = PublicProofParams::new_for_tests(192, 320, 256);
            p.seed_derivation = derivation;
            p.hash_a = [0xAA; 32];
            p.hash_b = [0xBB; 32];
            let (b, a) = p.commitment_hash(job_key);
            assert_eq!(b, expected_b, "{derivation:?} b_noise_seed changed — consensus break");
            assert_eq!(a, expected_a, "{derivation:?} a_noise_seed changed — consensus break");
        }
    }

    #[test]
    fn commitment_hash_pinned_vector_salted_moe() {
        let cases = [
            (
                SeedDerivation::Salted,
                // B's seed does not involve the routing fold, so it matches the
                // dense vector for the same derivation.
                [
                    0x60, 0xed, 0x9b, 0x73, 0xc5, 0xa9, 0x59, 0x9b, 0x20, 0x0b, 0x6c, 0xd5, 0x63, 0xe7, 0xf0, 0xd5, 0xd9, 0xa6,
                    0x7d, 0x24, 0x02, 0xd8, 0x5f, 0xd4, 0xef, 0x96, 0x6c, 0x58, 0x00, 0x80, 0xd0, 0xe5,
                ],
                [
                    0x26, 0x8a, 0x2e, 0xb7, 0x8b, 0x3b, 0x2d, 0x2f, 0xea, 0xd2, 0x62, 0x22, 0x1b, 0x24, 0xc6, 0x34, 0x6d, 0x0a,
                    0x6d, 0x20, 0x18, 0x90, 0xa4, 0x67, 0x75, 0x1f, 0x62, 0x3b, 0x7e, 0xac, 0xd5, 0xc2,
                ],
            ),
            (
                SeedDerivation::SaltedLattice,
                [
                    0x32, 0x84, 0xfc, 0xae, 0x62, 0x3a, 0x9b, 0xca, 0xfb, 0xb0, 0xae, 0x26, 0xcc, 0x41, 0xad, 0xca, 0x21, 0x3a,
                    0x62, 0x4f, 0xb5, 0x9a, 0xf3, 0xf6, 0xce, 0x22, 0xf9, 0x2d, 0x5c, 0x70, 0x4a, 0x5e,
                ],
                [
                    0x1e, 0x6e, 0x6a, 0xf7, 0xb2, 0x60, 0xe2, 0x23, 0x21, 0x74, 0x0b, 0x11, 0xf1, 0xb0, 0x0f, 0xc2, 0x0f, 0x96,
                    0xab, 0x66, 0xb8, 0x0d, 0xd6, 0x02, 0xf0, 0xe1, 0x90, 0xed, 0x7d, 0x15, 0x95, 0x0e,
                ],
            ),
        ];

        for (derivation, expected_b, expected_a) in cases {
            let mut p = PublicProofParams::new_for_tests(192, 320, 256);
            p.seed_derivation = derivation;
            p.hash_a = [0xAA; 32];
            p.hash_b = [0xBB; 32];
            p.moe = Some(MoEParams {
                expert_idx: 0,
                routing_offsets: vec![3, 5, 9, 12],
                hash_routing: [0xCC; 32],
                outer_indices: vec![],
            });

            let (b, a) = p.commitment_hash([0x11u8; 32]);
            assert_eq!(b, expected_b, "{derivation:?} MoE b_noise_seed changed — consensus break");
            assert_eq!(a, expected_a, "{derivation:?} MoE a_noise_seed changed — consensus break");
        }
    }
}

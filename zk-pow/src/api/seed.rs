//! Salted (certificate V3) Merkle-root pre-hashing and the seed-derivation
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
    /// the (unchanged) seed chain.
    Salted,
}

impl SeedDerivation {
    pub fn bind_roots(self, hash_a: &Hash256, hash_b: &Hash256, m: u32, n: u32) -> (Hash256, Hash256) {
        match self {
            Self::Legacy => (*hash_a, *hash_b),
            Self::Salted => (bind_root_a(hash_a, m), bind_root_b(hash_b, n)),
        }
    }
}

/// Domain-separation salt for A's root: `blake3("lattice/cert-v3/noise-seed/A")`.
/// Hardcoded so consensus doesn't depend on runtime string hashing (re-derived in a test).
pub const SEED_SALT_A: Hash256 = [
    0x15, 0xde, 0x6a, 0x03, 0xb7, 0x45, 0x9c, 0x19, 0x3c, 0x0a, 0x60, 0xf6, 0x09, 0x01, 0x0b, 0x7a, 0x30, 0xe9, 0xd2, 0x46, 0x06,
    0x39, 0x54, 0x38, 0xa1, 0x01, 0xe2, 0xdd, 0x9e, 0x5c, 0xe9, 0x8f,
];

/// Domain-separation salt for B's root: `blake3("lattice/cert-v3/noise-seed/B")`.
pub const SEED_SALT_B: Hash256 = [
    0x9c, 0xe3, 0x83, 0xb7, 0x8d, 0xe3, 0x57, 0xc0, 0x48, 0x77, 0x0f, 0xeb, 0xe8, 0x87, 0xb7, 0xd5, 0x7f, 0x0f, 0x12, 0xc8, 0x89,
    0x93, 0x95, 0xf1, 0xf5, 0x41, 0x0a, 0x63, 0xb3, 0xa2, 0xbe, 0x29,
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::api::proof::{MoEParams, PublicProofParams};

    #[test]
    fn seed_salts_match_context_strings() {
        assert_eq!(SEED_SALT_A, blake3_digest(b"lattice/cert-v3/noise-seed/A", None));
        assert_eq!(SEED_SALT_B, blake3_digest(b"lattice/cert-v3/noise-seed/B", None));
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
                    0x33, 0xf3, 0x80, 0xa5, 0xe3, 0x98, 0xdf, 0x8f, 0xab, 0xde, 0x03, 0xca, 0x8f, 0x97, 0x75, 0x21, 0x75, 0x0d,
                    0x44, 0x7c, 0x0a, 0xad, 0x1f, 0x0d, 0x63, 0x8f, 0x15, 0xe4, 0x69, 0x1f, 0xf9, 0x5b,
                ],
                [
                    0x2a, 0x8d, 0x51, 0x83, 0x9c, 0xd3, 0x5e, 0xa2, 0xe0, 0xd1, 0x47, 0xcb, 0xef, 0x5d, 0xdf, 0xd4, 0x00, 0x47,
                    0x7a, 0x98, 0xd1, 0x61, 0x94, 0x75, 0xa0, 0xcb, 0x32, 0x6c, 0x80, 0xc0, 0xf8, 0x88,
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
        let mut p = PublicProofParams::new_for_tests(192, 320, 256);
        p.seed_derivation = SeedDerivation::Salted;
        p.hash_a = [0xAA; 32];
        p.hash_b = [0xBB; 32];
        p.moe = Some(MoEParams {
            expert_idx: 0,
            routing_offsets: vec![3, 5, 9, 12],
            hash_routing: [0xCC; 32],
            outer_indices: vec![],
        });

        let (b, a) = p.commitment_hash([0x11u8; 32]);
        // B's seed does not involve the routing fold, so it matches the dense salted vector.
        let expected_b = [
            0x33, 0xf3, 0x80, 0xa5, 0xe3, 0x98, 0xdf, 0x8f, 0xab, 0xde, 0x03, 0xca, 0x8f, 0x97, 0x75, 0x21, 0x75, 0x0d, 0x44,
            0x7c, 0x0a, 0xad, 0x1f, 0x0d, 0x63, 0x8f, 0x15, 0xe4, 0x69, 0x1f, 0xf9, 0x5b,
        ];
        let expected_a = [
            0x11, 0xfd, 0xa5, 0xdd, 0xfa, 0x25, 0x38, 0x84, 0x59, 0xd8, 0x52, 0xa0, 0x6c, 0xe0, 0xf6, 0xd1, 0x42, 0x66, 0x17,
            0x52, 0x72, 0x82, 0x44, 0x63, 0x18, 0x78, 0x5a, 0x30, 0xf9, 0x53, 0x70, 0xc5,
        ];
        assert_eq!(b, expected_b, "salted MoE b_noise_seed changed — consensus break");
        assert_eq!(a, expected_a, "salted MoE a_noise_seed changed — consensus break");
    }
}

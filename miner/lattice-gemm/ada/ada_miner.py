"""Standalone proof-of-work miner for consumer (sm_80/sm_89) GPUs.

One attempt: commit to random A and B, derive the noise from the commitment,
run the noised GEMM with transcript hashing, and if a hash tile meets the
target, turn the hit into a PlainProof built from the *plain* A and B (the
verifier regenerates the noise itself).

The hash tile is a contiguous 16x16 block of C, i.e. rows_pattern and
cols_pattern are both [0, 1, ..., 15]. That satisfies the consensus bounds
(16x16 = 256 <= 256 elements, divisible by the 2-row circuit tile).
"""

from dataclasses import dataclass

import torch
from lattice_gateway.comm.dataclasses import MiningJob, OpenedBlockInfo
from lattice_gateway.comm.mining_configuration import LatticeMiningConfigurationFactory
from lattice_mining import PlainProof
from miner_base.block_submission import create_proof
from miner_base.commitment_hash import CommitmentHasher

import lattice_ada

RANK = 128
HASH_TILE = 16


def mining_config(k: int):
    idx = list(range(HASH_TILE))
    return LatticeMiningConfigurationFactory.create(
        common_dim=k, rank=RANK, row_indices=idx, col_indices=idx
    )


@dataclass
class Hit:
    opened: OpenedBlockInfo
    row: int
    col: int


def random_operands(m: int, n: int, k: int, device="cuda", generator=None):
    """int8 operands in the legal range [-64, 63]."""
    A = torch.randint(-64, 64, (m, k), dtype=torch.int8, device=device, generator=generator)
    Bt = torch.randint(-64, 64, (n, k), dtype=torch.int8, device=device, generator=generator)
    return A, Bt


def attempt(job: MiningJob, A: torch.Tensor, Bt: torch.Tensor, config: int | None = None) -> Hit | None:
    """Run one noised GEMM for `job` on operands (A: m x k, Bt: n x k)."""
    m, k = A.shape
    n = Bt.shape[0]
    cfg = mining_config(k)
    key = CommitmentHasher.get_key(job.incomplete_header_bytes, cfg)

    key_words = lattice_ada._words(key, A.device)
    root_a = lattice_ada.merkle_root(A, key_words)
    root_b = lattice_ada.merkle_root(Bt, key_words)
    commitment = CommitmentHasher.commitment_hash_from_merkle_roots(
        root_a,
        root_b,
        key,
        salted_dims=(m, n) if job.cert_version.uses_salted_seeds else None,
    )

    A_noised, Bt_noised = lattice_ada.noise_operands(
        A, Bt, commitment.noise_seed_A, commitment.noise_seed_B
    )
    target = job.adjust_target(mining_config=cfg)
    _, hit, _ = lattice_ada.pow_gemm(
        A_noised, Bt_noised, commitment.noise_seed_A, target, config=config
    )
    if hit is None:
        return None
    r, c = hit
    opened = OpenedBlockInfo(
        A_row_indices=list(range(r, r + HASH_TILE)),
        B_column_indices=list(range(c, c + HASH_TILE)),
        A=A.cpu(),  # host copies only on a hit
        B_t=Bt.cpu(),
        commitment_hash=commitment,
        noise_rank=RANK,
    )
    return Hit(opened, r, c)


def make_proof(job: MiningJob, hit: Hit) -> PlainProof:
    return create_proof(hit.opened, job.incomplete_header_bytes)

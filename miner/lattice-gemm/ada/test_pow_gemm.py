"""Checks the sm_89 PoW GEMM against the reference in miner-base/noisy_gemm.py.

The reference transcript logic (rotl-xor of per-hash-tile XOR reductions every
128 k, then keyed BLAKE3) is reproduced here without importing miner_base, so
the test only needs torch, numpy and blake3.
"""

import os
import struct

import blake3
import numpy as np
import pytest
import torch

import lattice_ada

pytestmark = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs a CUDA GPU")

RANK = 128
HT = 16


def rotl32(x, n):
    x &= 0xFFFFFFFF
    return ((x << n) | (x >> (32 - n))) & 0xFFFFFFFF


def reference(A, Bt):
    """Returns C (int32), transcripts[hi][wi] -> list of 16 uint32, mirroring
    NoisyGemm._process_output_tile / _accumulate_transcripts."""
    m, k = A.shape
    n = Bt.shape[0]
    A32, B32 = A.to(torch.int32), Bt.to(torch.int32)
    C = torch.zeros((m, n), dtype=torch.int32)
    th, tw = m // HT, n // HT
    tr = np.zeros((th, tw, 16), dtype=np.uint32)
    rc = 0
    for p in range(0, k, RANK):
        p_max = min(p + RANK, k)
        C += A32[:, p:p_max] @ B32[:, p:p_max].T
        if p_max - p == RANK:
            blk = C[: th * HT, : tw * HT].numpy().view(np.uint32)
            tiles = blk.reshape(th, HT, tw, HT).transpose(0, 2, 1, 3).reshape(th, tw, -1)
            h = np.bitwise_xor.reduce(tiles, axis=2)
            idx = rc % 16
            old = tr[:, :, idx].astype(np.uint64)
            tr[:, :, idx] = (((old << 13) | (old >> 19)) & 0xFFFFFFFF).astype(np.uint32) ^ h
            rc += 1
    return C, tr


def tile_hash(words, key):
    data = b"".join(struct.pack("<I", int(w)) for w in words)
    return int.from_bytes(blake3.blake3(data, key=key).digest(), "little")


def rand_i8(*shape, seed):
    g = torch.Generator().manual_seed(seed)
    return torch.randint(-64, 64, shape, generator=g, dtype=torch.int8)


SHAPES = [
    (128, 128, 128),
    (256, 384, 1024),
    (96, 160, 512),
    (130, 150, 400),  # m, n not multiples of 16; k not a multiple of 128
    (64, 64, 176),  # partial last k step
    (16, 16, 128),
    (512, 512, 4096),
]


@pytest.mark.parametrize("config", [0, 1])
@pytest.mark.parametrize("m,n,k", SHAPES)
def test_matches_reference(m, n, k, config):
    A = rand_i8(m, k, seed=1)
    Bt = rand_i8(n, k, seed=2)
    key = bytes(range(32))
    C_ref, tr_ref = reference(A, Bt)
    C, hit, tr = lattice_ada.pow_gemm(
        A.cuda(), Bt.cuda(), key, 0, config=config, want_transcripts=True
    )
    assert torch.equal(C.cpu(), C_ref)
    got = tr.numpy().view(np.uint32)
    if k >= RANK:
        assert np.array_equal(got, tr_ref)
    assert hit is None  # target 0 is unreachable


@pytest.mark.parametrize("config", [0, 1])
def test_target_check(config):
    m, n, k = 256, 256, 1024
    A, Bt = rand_i8(m, k, seed=3), rand_i8(n, k, seed=4)
    key = os.urandom(32)
    _, tr_ref = reference(A, Bt)
    hashes = {
        (hi, wi): tile_hash(tr_ref[hi, wi], key)
        for hi in range(m // HT)
        for wi in range(n // HT)
    }
    best = min(hashes.values())
    # Target equal to the best hash: the GPU must find a tile, and the tile it
    # reports must really satisfy the target.
    _, hit, _ = lattice_ada.pow_gemm(A.cuda(), Bt.cuda(), key, best, config=config)
    assert hit is not None
    assert hashes[(hit[0] // HT, hit[1] // HT)] <= best
    # One below the best: nothing may be reported.
    _, hit, _ = lattice_ada.pow_gemm(A.cuda(), Bt.cuda(), key, best - 1, config=config)
    assert hit is None
    # Everything passes.
    _, hit, _ = lattice_ada.pow_gemm(A.cuda(), Bt.cuda(), key, 2**256 - 1, config=config)
    assert hit is not None

"""Mine on the GPU, build a PlainProof, and have the Rust verifier accept it."""

import pytest
import torch
from lattice_gateway.blockchain_utils.zk_certificate import CertificateVersion
from lattice_gateway.comm.dataclasses import MiningJob
from lattice_mining import IncompleteBlockHeader, verify_plain_proof_for_cert_version

import ada_miner

pytestmark = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs a CUDA GPU")


def header(nbits):
    return IncompleteBlockHeader(
        version=0x20000000,
        prev_block=bytes(range(32)),
        merkle_root=bytes(range(32, 64)),
        timestamp=1_800_000_000,
        nbits=nbits,
    )


def mine(cert_version, nbits, target, m=256, n=256, k=2048, tries=200):
    h = header(nbits)
    job = MiningJob(bytes(h.to_bytes()), target, cert_version)
    gen = torch.Generator(device="cuda").manual_seed(7)
    for i in range(tries):
        A, Bt = ada_miner.random_operands(m, n, k, generator=gen)
        hit = ada_miner.attempt(job, A, Bt)
        if hit:
            return job, h, hit, i
    return job, h, None, tries


@pytest.mark.parametrize("cert", [CertificateVersion.ZK_V3])
def test_verifier_accepts_gpu_proof(cert):
    # Easy enough that about half of all tiles hit: the job target is multiplied by
    # tile size (256) * k/rank (16) * 128 = 2^19, so 2^236 becomes 2^255.
    nbits = 0x1E100000
    target = 1 << 236
    job, h, hit, tries = mine(cert, nbits, target)
    assert hit is not None, "no tile met the target"
    proof = ada_miner.make_proof(job, hit)
    ok, msg = verify_plain_proof_for_cert_version(int(cert), h, proof, nbits)
    assert ok, msg


def _mined():
    nbits, target = 0x1E100000, 1 << 236
    job, h, hit, _ = mine(CertificateVersion.ZK_V3, nbits, target)
    assert hit is not None
    return job, h, ada_miner.make_proof(job, hit), nbits


def test_verifier_rejects_wrong_header():
    job, h, proof, nbits = _mined()
    other = header(nbits)
    bad = IncompleteBlockHeader(
        version=0x20000000,
        prev_block=bytes(range(32)),
        merkle_root=bytes(range(32, 64)),
        timestamp=1_800_000_001,
        nbits=nbits,
    )
    ok, _ = verify_plain_proof_for_cert_version(int(CertificateVersion.ZK_V3), bad, proof, nbits)
    assert not ok


def test_verifier_rejects_much_harder_target():
    job, h, proof, nbits = _mined()
    harder = 0x1A010000  # 2^200: a random tile essentially never qualifies
    ok, _ = verify_plain_proof_for_cert_version(int(CertificateVersion.ZK_V3), h, proof, harder)
    assert not ok


def test_verifier_rejects_unsalted_derivation():
    job, h, proof, nbits = _mined()
    ok, _ = verify_plain_proof_for_cert_version(int(CertificateVersion.ZK_MOE), h, proof, nbits)
    assert not ok

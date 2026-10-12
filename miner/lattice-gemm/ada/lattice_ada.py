"""ctypes wrapper for the sm_89 PoW GEMM (lattice_ada.cu).

Build the shared library first:  make -C miner/lattice-gemm/ada
"""

import ctypes
import os

import torch

_HERE = os.path.dirname(os.path.abspath(__file__))
_LIB = None


def _lib():
    global _LIB
    if _LIB is None:
        path = os.environ.get("LATTICE_ADA_LIB", os.path.join(_HERE, "liblattice_ada.so"))
        _LIB = ctypes.CDLL(path)
        _LIB.lattice_ada_pow_gemm.restype = ctypes.c_int
        _LIB.lattice_ada_pow_gemm.argtypes = [ctypes.c_void_p] * 3 + [ctypes.c_int] * 3 + [
            ctypes.c_void_p
        ] * 4 + [ctypes.c_int, ctypes.c_void_p]
    return _LIB


def words_le(value: int) -> torch.Tensor:
    """uint256 -> 8 little-endian uint32 words, stored in an int32 tensor."""
    ws = [(value >> (32 * i)) & 0xFFFFFFFF for i in range(8)]
    return torch.tensor([w - (1 << 32) if w >= 1 << 31 else w for w in ws], dtype=torch.int32)


def pow_gemm(A, Bt, key_bytes: bytes, target: int, config: int = 1, want_transcripts=False):
    """C = A @ Bt.T with PoW transcript hashing.

    A: (m, k) int8 CUDA, Bt: (n, k) int8 CUDA (B stored K-major), k % 16 == 0.
    Returns (C int32 (m, n), found (row, col) or None, transcripts or None).
    """
    assert A.dtype == torch.int8 and Bt.dtype == torch.int8 and A.is_cuda and Bt.is_cuda
    A = A.contiguous()
    Bt = Bt.contiguous()
    m, k = A.shape
    n, k2 = Bt.shape
    assert k == k2
    dev = A.device
    C = torch.empty((m, n), dtype=torch.int32, device=dev)
    key = torch.tensor(
        [int.from_bytes(key_bytes[4 * i : 4 * i + 4], "little", signed=True) for i in range(8)],
        dtype=torch.int32,
        device=dev,
    )
    tgt = words_le(target).to(dev)
    found = torch.zeros(3, dtype=torch.int32, device=dev)
    tr = None
    if want_transcripts:
        tr = torch.zeros((m // 16) * (n // 16) * 16, dtype=torch.int32, device=dev)
    rc = _lib().lattice_ada_pow_gemm(
        A.data_ptr(),
        Bt.data_ptr(),
        C.data_ptr(),
        m,
        n,
        k,
        key.data_ptr(),
        tgt.data_ptr(),
        found.data_ptr(),
        tr.data_ptr() if tr is not None else None,
        config,
        torch.cuda.current_stream(dev).cuda_stream,
    )
    if rc != 0:
        raise RuntimeError(f"lattice_ada_pow_gemm failed: cudaError {rc}")
    f = found.cpu().tolist()
    hit = (f[1], f[2]) if f[0] == 2 else None
    if tr is not None:
        tr = tr.view(torch.int32).view(m // 16, n // 16, 16).cpu()
    return C, hit, tr

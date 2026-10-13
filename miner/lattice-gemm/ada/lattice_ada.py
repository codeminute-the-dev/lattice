"""ctypes wrapper for the PoW GEMM (lattice_ada.cu), any NVIDIA GPU from Pascal up.

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
        _LIB.lattice_ada_best_config.restype = ctypes.c_int
        _LIB.lattice_ada_best_config.argtypes = []
        _LIB.lattice_ada_blake3_scratch_words.restype = ctypes.c_size_t
        _LIB.lattice_ada_blake3_scratch_words.argtypes = [ctypes.c_size_t]
        _LIB.lattice_ada_blake3_keyed.restype = ctypes.c_int
        _LIB.lattice_ada_blake3_keyed.argtypes = [
            ctypes.c_void_p,
            ctypes.c_size_t,
            ctypes.c_void_p,
            ctypes.c_void_p,
            ctypes.c_void_p,
            ctypes.c_void_p,
        ]
        _LIB.lattice_ada_noise_uniform.restype = ctypes.c_int
        _LIB.lattice_ada_noise_uniform.argtypes = [ctypes.c_void_p] * 3 + [
            ctypes.c_int,
            ctypes.c_void_p,
        ]
        _LIB.lattice_ada_noise_perm.restype = ctypes.c_int
        _LIB.lattice_ada_noise_perm.argtypes = [ctypes.c_void_p] * 4 + [
            ctypes.c_int,
            ctypes.c_void_p,
        ]
        _LIB.lattice_ada_apply_noise.restype = ctypes.c_int
        _LIB.lattice_ada_apply_noise.argtypes = [ctypes.c_void_p] * 5 + [
            ctypes.c_int,
            ctypes.c_int,
            ctypes.c_void_p,
        ]
    return _LIB


def words_le(value: int) -> torch.Tensor:
    """uint256 -> 8 little-endian uint32 words, stored in an int32 tensor."""
    ws = [(value >> (32 * i)) & 0xFFFFFFFF for i in range(8)]
    return torch.tensor([w - (1 << 32) if w >= 1 << 31 else w for w in ws], dtype=torch.int32)


def best_config(device=None) -> int:
    """Kernel for a device: 1 = tensor cores (sm_80+), 2 = dp4a (sm_61+)."""
    forced = os.environ.get("LATTICE_ADA_CONFIG")  # 0/1/2, to force a kernel
    if forced:
        return int(forced)
    if device is not None:
        torch.cuda.set_device(device)
    c = _lib().lattice_ada_best_config()
    if c < 0:
        raise RuntimeError("GPU too old: the miner needs compute capability >= 6.1 (GTX 10xx or newer)")
    return c


def pow_gemm(A, Bt, key_bytes: bytes, target: int, config: int | None = None, want_transcripts=False):
    """C = A @ Bt.T with PoW transcript hashing.

    A: (m, k) int8 CUDA, Bt: (n, k) int8 CUDA (B stored K-major), k % 16 == 0.
    config None picks the kernel for the device (best_config).
    Returns (C int32 (m, n), found (row, col) or None, transcripts or None).
    """
    assert A.dtype == torch.int8 and Bt.dtype == torch.int8 and A.is_cuda and Bt.is_cuda
    A = A.contiguous()
    Bt = Bt.contiguous()
    m, k = A.shape
    n, k2 = Bt.shape
    assert k == k2
    dev = A.device
    if config is None:
        config = best_config(dev)
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


NOISE_RANK = 128
_SEED_A = b"A_tensor" + b"\x00" * 24
_SEED_B = b"B_tensor" + b"\x00" * 24


def _words(b: bytes, dev) -> torch.Tensor:
    assert len(b) == 32
    return torch.tensor(
        [int.from_bytes(b[4 * i : 4 * i + 4], "little", signed=True) for i in range(8)],
        dtype=torch.int32,
        device=dev,
    )


def _check(rc, what):
    if rc != 0:
        raise RuntimeError(f"{what} failed: cudaError {rc}")


def _stream(dev):
    return torch.cuda.current_stream(dev).cuda_stream


def noise_uniform(key: bytes, seed: bytes, rows: int, dev) -> torch.Tensor:
    """(rows, 128) int8 uniform noise, matching NoiseGenerator.__generate_uniform_random_matrix."""
    out = torch.empty((rows, NOISE_RANK), dtype=torch.int8, device=dev)
    k, s = _words(key, dev), _words(seed, dev)
    _check(
        _lib().lattice_ada_noise_uniform(
            k.data_ptr(), s.data_ptr(), out.data_ptr(), rows, _stream(dev)
        ),
        "noise_uniform",
    )
    return out


def noise_perm(key: bytes, seed: bytes, k: int, dev):
    """(p, q) int32[k]: line c has +1 at p[c] and -1 at q[c]."""
    p = torch.empty(k, dtype=torch.int32, device=dev)
    q = torch.empty(k, dtype=torch.int32, device=dev)
    kk, s = _words(key, dev), _words(seed, dev)
    _check(
        _lib().lattice_ada_noise_perm(
            kk.data_ptr(), s.data_ptr(), p.data_ptr(), q.data_ptr(), k, _stream(dev)
        ),
        "noise_perm",
    )
    return p, q


def apply_noise(X, L, p, q) -> torch.Tensor:
    """out[i, c] = X[i, c] + L[i, p[c]] - L[i, q[c]] as int8."""
    X = X.contiguous()
    rows, k = X.shape
    out = torch.empty_like(X)
    _check(
        _lib().lattice_ada_apply_noise(
            X.data_ptr(), L.data_ptr(), p.data_ptr(), q.data_ptr(), out.data_ptr(), rows, k,
            _stream(X.device),
        ),
        "apply_noise",
    )
    return out


def noise_operands(A, Bt, key_A: bytes, key_B: bytes):
    """Noised A (m, k) and noised Bt (n, k) for the given commitment seeds."""
    dev = A.device
    m, k = A.shape
    n = Bt.shape[0]
    EAL = noise_uniform(key_A, _SEED_A, m, dev)
    pa, qa = noise_perm(key_A, _SEED_A, k, dev)
    EBR = noise_uniform(key_B, _SEED_B, n, dev)
    pb, qb = noise_perm(key_B, _SEED_B, k, dev)
    return apply_noise(A, EAL, pa, qa), apply_noise(Bt, EBR, pb, qb)


def merkle_root(t: torch.Tensor, key_words: torch.Tensor) -> bytes:
    """Merkle root (keyed BLAKE3 of the zero-padded bytes) of an int8 CUDA tensor.

    key_words: the 32-byte key as 8 int32 words on the same device (see `_words`).
    """
    t = t.contiguous()
    nbytes = t.numel()
    lib = _lib()
    scratch = torch.empty(lib.lattice_ada_blake3_scratch_words(nbytes), dtype=torch.int32, device=t.device)
    out = torch.empty(8, dtype=torch.int32, device=t.device)
    _check(
        lib.lattice_ada_blake3_keyed(
            t.data_ptr(), nbytes, key_words.data_ptr(), scratch.data_ptr(), out.data_ptr(),
            _stream(t.device),
        ),
        "blake3_keyed",
    )
    return out.cpu().numpy().tobytes()

from collections import deque
from dataclasses import dataclass
from threading import Lock, Semaphore

import torch
from blake3 import blake3
from loguru import logger
from lattice_gemm_cuda import HostSignalHeader, get_host_signal_header_size

SIZE_U32 = 4
BLAKE3_DIGEST_SIZE_U32 = blake3.digest_size // SIZE_U32


def make_pow_target_tensor(value: int, device="cuda") -> torch.Tensor:
    """Create a pow_target tensor from a uint256 integer value."""

    result = torch.empty((BLAKE3_DIGEST_SIZE_U32,), dtype=torch.uint32, device=device)
    for i in range(BLAKE3_DIGEST_SIZE_U32):
        result[i] = value & 0xFFFFFFFF
        value >>= 32
    return result


_LOGGER = logger.bind(name=__name__)


class HostSignalHeaderPinnedPool:
    def __init__(self, pool_size: int = 128) -> None:
        self._pool_size = pool_size
        self._available_buffers = deque()
        self._used_buffers: set[int] = set()
        self._lock = Lock()
        self._semaphore = Semaphore(self._pool_size)

        host_signal_header_size = get_host_signal_header_size()

        # pre-allocate pinned buffer
        for _ in range(self._pool_size):
            self._available_buffers.append(
                torch.zeros((host_signal_header_size,), dtype=torch.int8, pin_memory=True),
            )

    def acquire(self) -> torch.Tensor:
        if not self._available_buffers:
            _LOGGER.warning(f"Pool size exceeded, {self._pool_size=}")

        self._semaphore.acquire()

        with self._lock:
            buffer = self._available_buffers.popleft()
            if id(buffer) in self._used_buffers:
                raise AssertionError("Unexpectedly found available buffer in _used_buffers")
            self._used_buffers.add(id(buffer))
            return buffer

    def release(self, buffer: torch.Tensor) -> None:
        with self._lock:
            if id(buffer) not in self._used_buffers:
                raise ValueError("Attempted to release unused buffer")
            self._used_buffers.remove(id(buffer))
            self._available_buffers.append(buffer)
            buffer.zero_()

        self._semaphore.release()


@dataclass
class ProofTileIndices:
    A_row_indices: list[int]
    B_column_indices: list[int]


def extract_indices(header: HostSignalHeader) -> ProofTileIndices:
    row_tile_coord = header.tileCoord[0] * header.mma_tile_size.m
    col_tile_coord = header.tileCoord[1] * header.mma_tile_size.n
    thread_rows = sorted(set(header.thread_rows))
    thread_cols = sorted(set(header.thread_cols))

    return ProofTileIndices(
        A_row_indices=[row_tile_coord + r for r in thread_rows],
        B_column_indices=[col_tile_coord + c for c in thread_cols],
    )

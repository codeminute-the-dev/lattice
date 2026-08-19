import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from unittest.mock import patch

import pytest
import torch
from loguru_caplog import loguru_caplog as caplog  # noqa: F401
from lattice_gemm import HostSignalHeaderPinnedPool


def create_pool(pool_size: int = 5) -> HostSignalHeaderPinnedPool:
    """Helper function to create a pool with mocked header size."""
    with patch("lattice_gemm.helpers.get_host_signal_header_size", return_value=64):
        return HostSignalHeaderPinnedPool(pool_size)


class TestHostSignalHeaderPinnedPool:
    def test_init_creates_pool_with_correct_size(self):
        """Test that pool is initialized with correct number of buffers."""
        pool_size = 10
        pool = create_pool(pool_size)

        assert pool._pool_size == pool_size
        assert len(pool._available_buffers) == pool_size
        assert len(pool._used_buffers) == 0
        assert pool._semaphore._value == pool_size

    def test_acquire_returns_pinned_tensor(self):
        """Test that acquire returns a pinned PyTorch tensor."""
        pool = create_pool(5)

        buffer = pool.acquire()

        assert isinstance(buffer, torch.Tensor)
        assert buffer.is_pinned()
        assert buffer.dtype == torch.int8
        assert len(pool._available_buffers) == 4
        assert len(pool._used_buffers) == 1
        assert id(buffer) in pool._used_buffers

    def test_release_returns_buffer_to_pool(self):
        """Test that release properly returns buffer to available pool."""
        pool = create_pool(5)

        buffer = pool.acquire()
        buffer.fill_(42)

        pool.release(buffer)

        assert len(pool._available_buffers) == 5
        assert len(pool._used_buffers) == 0
        assert id(buffer) not in pool._used_buffers

    def test_acquire_release_cycle(self):
        """Test multiple acquire/release cycles work correctly."""
        pool = create_pool(3)

        buffers = []

        for _ in range(3):
            buffers.append(pool.acquire())

        assert len(pool._available_buffers) == 0
        assert len(pool._used_buffers) == 3

        for buffer in buffers:
            pool.release(buffer)

        assert len(pool._available_buffers) == 3
        assert len(pool._used_buffers) == 0

    def test_release_unused_buffer_raises_error(self):
        """Test that releasing an unused buffer raises ValueError."""
        pool = create_pool(5)

        fake_buffer = torch.zeros(64, dtype=torch.int8)

        with pytest.raises(ValueError, match="Attempted to release unused buffer"):
            pool.release(fake_buffer)

    def test_double_release_raises_error(self):
        """Test that releasing the same buffer twice raises ValueError."""
        pool = create_pool(5)

        buffer = pool.acquire()
        pool.release(buffer)

        with pytest.raises(ValueError, match="Attempted to release unused buffer"):
            pool.release(buffer)

    def test_pool_exhaustion_warning(self, caplog):  # noqa: F811
        """test that warning is logged when pool is exhausted."""
        pool = create_pool(2)

        buffer1 = pool.acquire()
        pool.acquire()

        with pool._lock:
            pool._available_buffers.clear()

        def acquire_with_timeout():
            return pool.acquire()

        thread = threading.Thread(target=acquire_with_timeout)
        thread.start()

        time.sleep(0.1)

        pool.release(buffer1)
        thread.join()

        assert "Pool size exceeded" in caplog.text

    def test_thread_safety_concurrent_acquire_release(self):
        """Test that pool is thread-safe under concurrent access."""
        pool = create_pool(20)

        def worker():
            buffers = []
            for _ in range(5):
                buffers.append(pool.acquire())

            time.sleep(0.001)

            for buffer in buffers:
                pool.release(buffer)

            return len(buffers)

        with ThreadPoolExecutor(max_workers=10) as executor:
            futures = [executor.submit(worker) for _ in range(10)]
            results = [future.result() for future in as_completed(futures)]

        assert all(result == 5 for result in results)

        assert len(pool._available_buffers) == 20
        assert len(pool._used_buffers) == 0

    def test_semaphore_blocks_when_pool_exhausted(self):
        """Test that semaphore properly blocks when pool is exhausted."""
        pool = create_pool(2)

        buffer1 = pool.acquire()
        pool.acquire()

        acquire_completed = threading.Event()
        acquired_buffer = None

        def try_acquire():
            nonlocal acquired_buffer
            acquired_buffer = pool.acquire()
            acquire_completed.set()

        thread = threading.Thread(target=try_acquire)
        thread.start()

        assert not acquire_completed.wait(timeout=0.1)

        pool.release(buffer1)

        assert acquire_completed.wait(timeout=1.0)
        thread.join()

        assert acquired_buffer is not None
        assert isinstance(acquired_buffer, torch.Tensor)

    def test_buffer_properties_correct_size_and_type(self):
        """Test that buffers have correct size and properties."""
        header_size = 128
        with patch("lattice_gemm.helpers.get_host_signal_header_size", return_value=header_size):
            pool = HostSignalHeaderPinnedPool(5)

        buffer = pool.acquire()

        assert buffer.shape == (header_size,)
        assert buffer.dtype == torch.int8
        assert buffer.is_pinned()

        pool.release(buffer)

    def test_buffer_zeroed_after_release(self):
        """Test that buffers are properly zeroed after release."""
        pool = create_pool(1)  # Use pool size of 1 to guarantee same buffer

        buffer1 = pool.acquire()
        buffer1.fill_(123)
        assert torch.all(buffer1 == 123)

        pool.release(buffer1)

        buffer2 = pool.acquire()
        assert torch.all(buffer2 == 0)

        pool.release(buffer2)

    def test_pool_state_consistency_after_operations(self):
        """Test that pool maintains consistent state after various operations."""
        pool = create_pool(5)

        initial_available = len(pool._available_buffers)
        initial_used = len(pool._used_buffers)

        buffers = []
        for _ in range(3):
            buffers.append(pool.acquire())

        assert len(pool._available_buffers) == initial_available - 3
        assert len(pool._used_buffers) == initial_used + 3

        for _ in range(2):
            pool.release(buffers.pop())

        assert len(pool._available_buffers) == initial_available - 1
        assert len(pool._used_buffers) == initial_used + 1

        pool.release(buffers.pop())

        assert len(pool._available_buffers) == initial_available
        assert len(pool._used_buffers) == initial_used

    def test_assertion_error_on_corrupted_state(self):
        """Test that AssertionError is raised if internal state is corrupted."""
        pool = create_pool(5)

        available_buffer = pool._available_buffers[0]
        pool._used_buffers.add(id(available_buffer))

        with pytest.raises(
            AssertionError, match="Unexpectedly found available buffer in _used_buffers"
        ):
            pool.acquire()

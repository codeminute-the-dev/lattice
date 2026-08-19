import numpy as np
import pytest
import torch
from miner_base.inner_hash import hash_tile
from lattice_gemm.test_components import inner_hash as inner_hash_cuda


class TestInnerHash:
    """Test class for inner_hash function comparing CUDA implementation with Python reference."""

    def setUp(self):
        """Set up test fixtures."""
        torch.manual_seed(0)
        np.random.seed(0)

    def convert_for_reference(self, tensor_uint32: torch.Tensor) -> torch.Tensor:
        """Convert uint32 CUDA tensor to int32 CPU tensor for reference implementation."""
        # Convert to int32 for reference (Python reference expects int32)
        return tensor_uint32.cpu().to(torch.int32)

    def convert_for_cuda(self, tensor_int32: torch.Tensor) -> torch.Tensor:
        """Convert int32 CPU tensor to uint32 CUDA tensor for CUDA implementation."""
        # Convert to uint32 for CUDA (CUDA implementation expects uint32)
        return tensor_int32.to(torch.uint32).cuda()

    def create_test_tensors(self):
        """Create 3 different test tensors for testing."""
        # Test tensor 1: Sequential values (0, 1, 2, ..., 255)
        tensor1_int32 = torch.arange(256, dtype=torch.int32)
        tensor1_uint32 = self.convert_for_cuda(tensor1_int32)

        # Test tensor 2: Random values
        torch.manual_seed(42)  # For reproducibility
        tensor2_int32 = torch.randint(
            0, 2**30, (256,), dtype=torch.int32
        )  # Use smaller range to avoid overflow
        tensor2_uint32 = self.convert_for_cuda(tensor2_int32)

        # Test tensor 3: Mixed pattern with some structure
        tensor3_int32 = torch.zeros(256, dtype=torch.int32)
        # Fill with a pattern: alternating values, some zeros, some large values
        for i in range(256):
            if i % 3 == 0:
                tensor3_int32[i] = i * 1000
            elif i % 3 == 1:
                tensor3_int32[i] = 0
            else:
                tensor3_int32[i] = (i * 37) % 65536
        tensor3_uint32 = self.convert_for_cuda(tensor3_int32)

        return [
            (tensor1_uint32, tensor1_int32, "sequential"),
            (tensor2_uint32, tensor2_int32, "random"),
            (tensor3_uint32, tensor3_int32, "mixed_pattern"),
        ]

    def compare_results(self, cuda_result: torch.Tensor, reference_result: int, err_msg: str):
        cuda_result_cpu = cuda_result.cpu().item()
        np.testing.assert_equal(
            cuda_result_cpu,
            reference_result,
            err_msg=err_msg,
        )

    @pytest.mark.parametrize(
        "tensor_idx,test_name", [(0, "sequential"), (1, "random"), (2, "mixed_pattern")]
    )
    @pytest.mark.parametrize("size", [64, 256])
    def test_inner_hash_cuda_vs_reference(self, tensor_idx, test_name, size):
        """Test that CUDA implementation matches Python reference implementation."""
        if not torch.cuda.is_available():
            pytest.skip("CUDA not available")

        # Get test tensors
        test_tensors = self.create_test_tensors()
        tensor_uint32, tensor_int32, name = test_tensors[tensor_idx]

        # Run CUDA implementation
        cuda_result = inner_hash_cuda(tensor_uint32[:size])

        # Run Python reference implementation
        reference_result = hash_tile(tensor_int32[:size]).hash

        # Convert results for comparison
        self.compare_results(
            cuda_result,
            reference_result,
            f"CUDA and reference results don't match for {test_name} tensor",
        )

        # Additional checks on output properties
        assert cuda_result.shape == (1,), (
            f"CUDA result should have shape (1,), got {cuda_result.shape}"
        )
        assert cuda_result.dtype == torch.uint32, (
            f"CUDA result should have dtype uint32, got {cuda_result.dtype}"
        )

    def test_inner_hash_deterministic(self):
        """Test that the inner_hash function produces deterministic results."""
        if not torch.cuda.is_available():
            pytest.skip("CUDA not available")

        # Use the sequential tensor for determinism test
        test_tensors = self.create_test_tensors()
        tensor_uint32, _, _ = test_tensors[0]  # Use sequential tensor

        # Run multiple times
        results = []
        for _ in range(5):
            result = inner_hash_cuda(tensor_uint32)
            results.append(result.cpu().numpy())

        # Check that all results are identical
        for i in range(1, len(results)):
            np.testing.assert_array_equal(
                results[0],
                results[i],
                err_msg=f"Results are not deterministic: run 0 vs run {i}",
            )

        print(f"✓ Deterministic test passed - consistent result: {results[0]}")

    def test_inner_hash_edge_cases(self):
        """Test edge cases for the inner_hash function."""
        if not torch.cuda.is_available():
            pytest.skip("CUDA not available")

        # Test all zeros
        zeros_tensor = torch.zeros(256, dtype=torch.uint32, device="cuda")
        zeros_result = inner_hash_cuda(zeros_tensor)
        zeros_reference = hash_tile(torch.zeros(256, dtype=torch.int32)).hash

        self.compare_results(zeros_result, zeros_reference, "All zeros test failed")

        # Test all ones
        ones_tensor = torch.ones(256, dtype=torch.uint32, device="cuda")
        ones_result = inner_hash_cuda(ones_tensor)
        ones_reference = hash_tile(torch.ones(256, dtype=torch.int32)).hash

        self.compare_results(ones_result, ones_reference, "All ones test failed")

        # Test maximum values (within int32 range to avoid overflow in reference)
        max_safe_val = 2**30 - 1  # Safe for int32
        max_tensor = torch.full((256,), max_safe_val, dtype=torch.uint32, device="cuda")
        max_result = inner_hash_cuda(max_tensor)
        max_reference = hash_tile(torch.full((256,), max_safe_val, dtype=torch.int32)).hash

        self.compare_results(max_result, max_reference, "Maximum values test failed")

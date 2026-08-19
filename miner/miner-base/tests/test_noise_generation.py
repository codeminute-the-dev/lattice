import secrets

import pytest
import torch
from miner_base.noise_generation import NoiseGenerator


@pytest.fixture
def test_key() -> bytes:
    """Generate a 32-byte random key for BLAKE3 keyed hashing."""
    return secrets.token_bytes(32)


@pytest.fixture
def test_seed() -> bytes:
    """Generate a 32-byte random seed for BLAKE3 keyed hashing."""
    return secrets.token_bytes(32)


@pytest.fixture
def alt_test_key() -> bytes:
    """Generate an alternative 32-byte random key for testing different keys."""
    return secrets.token_bytes(32)


@pytest.fixture
def fixed_key() -> bytes:
    """Fixed 32-byte key for reproducible golden tests."""
    return b"golden_test_key_32_bytes_long_!!"


@pytest.fixture
def small_generator() -> NoiseGenerator:
    """Small NoiseGenerator for basic tests."""
    return NoiseGenerator(noise_rank=32, noise_range=32)


@pytest.fixture
def medium_generator() -> NoiseGenerator:
    """Medium NoiseGenerator for most tests."""
    return NoiseGenerator(noise_rank=64, noise_range=128)


@pytest.fixture
def large_generator() -> NoiseGenerator:
    """Large NoiseGenerator for performance tests."""
    return NoiseGenerator(noise_rank=128, noise_range=128)


def validate_permutation_matrix_properties(matrix: torch.Tensor, dim: int) -> None:
    """Validate that a matrix is a permutation matrix."""
    # Check 1) only 0, 1, -1 values;
    abs_matrix = matrix.abs()
    assert torch.all(abs_matrix <= 1)
    # Check 2) for each row/col, exactly two nonzero entries
    assert torch.all(abs_matrix.sum(dim=dim) == 2)
    # Check 3) for each row/col, number of 1s equals number of -1s
    sum_pos = (matrix == 1).sum(dim=dim)
    sum_neg = (matrix == -1).sum(dim=dim)
    assert torch.all(sum_pos == sum_neg), (
        f"Number of 1 and -1 does not match along dim={dim}. sum_pos={sum_pos}, sum_neg={sum_neg}"
    )


def validate_uniform_random_matrix_properties(matrix: torch.Tensor, noise_range: int) -> None:
    """Validate that a matrix is a uniform random matrix."""
    zero_point_translation = noise_range // 2
    min_noise_value = -zero_point_translation
    max_noise_value = noise_range - zero_point_translation
    assert torch.all(matrix >= min_noise_value) and torch.all(matrix <= max_noise_value), (
        f"noise must be in the range [{min_noise_value}, {max_noise_value}]"
    )


def validate_matrix_product_properties(
    mat1: torch.Tensor, mat2: torch.Tensor, noise_range: int
) -> None:
    """Validate that a matrix product is within range."""
    product = torch.matmul(mat1, mat2)
    validate_uniform_random_matrix_properties(product, noise_range)


class TestNoiseGeneratorCore:
    """Test suite for NoiseGenerator core functionality that actually works."""

    def test_uniform_random_matrix_generation(self, small_generator, test_key, test_seed):
        """Test uniform random matrix generation directly."""
        A_L = small_generator._NoiseGenerator__generate_uniform_random_matrix(
            test_seed,
            test_key,
            10,
        )
        B_R = small_generator._NoiseGenerator__generate_uniform_random_matrix(
            test_seed,
            test_key,
            12,
        )

        # Check dimensions and types
        assert A_L.shape == (10, 32)
        assert B_R.shape == (12, 32)
        assert A_L.dtype == torch.int8
        assert B_R.dtype == torch.int8

        validate_uniform_random_matrix_properties(A_L, noise_range=32)
        validate_uniform_random_matrix_properties(B_R, noise_range=32)

    @pytest.mark.parametrize("range_val", [16, 32, 64])
    def test_value_ranges_different_parameters(self, test_key, range_val, test_seed):
        """Test value ranges with different parameters."""

        generator = NoiseGenerator(noise_rank=32, noise_range=range_val)
        matrix = generator._NoiseGenerator__generate_uniform_random_matrix(
            test_seed,
            test_key,
            20,
        )

        validate_uniform_random_matrix_properties(matrix, noise_range=range_val)

    def test_permutation_matrix_basic_structure(self, small_generator, test_key, test_seed):
        """Test basic permutation matrix structure."""
        A_R = small_generator._NoiseGenerator__generate_permutation_matrix(
            test_seed,
            test_key,
            32,
            10,
            assign_columns=True,
        )
        B_L = small_generator._NoiseGenerator__generate_permutation_matrix(
            test_seed,
            test_key,
            10,
            32,
            assign_columns=False,
        )

        # Check dimensions and types
        assert A_R.shape == (32, 10)
        assert B_L.shape == (10, 32)
        assert A_R.dtype == torch.int8
        assert B_L.dtype == torch.int8

        validate_permutation_matrix_properties(A_R, dim=0)
        validate_permutation_matrix_properties(B_L, dim=1)


class TestNoiseGeneratorMatrixProperties:
    """Test suite for matrix property validation (limited tests)."""

    def test_uniform_matrix_diversity(self, large_generator, test_key, test_seed):
        """Test that uniform matrices have value diversity."""
        # Generate a reasonably large matrix
        matrix = large_generator._NoiseGenerator__generate_uniform_random_matrix(
            test_seed,
            test_key,
            150,
        )

        # Should have multiple unique values
        unique_values = torch.unique(matrix)
        assert len(unique_values) > 30, "Matrix should have diverse values"


class TestNoiseGeneratorFullFunctionality:
    """Test suite for the complete generate_noise_metrices functionality."""

    def validate_noise_metrices(
        self,
        A_L: torch.Tensor,
        A_R: torch.Tensor,
        B_L: torch.Tensor,
        B_R: torch.Tensor,
        noise_range: int,
    ) -> None:
        """Validate that a noise metrices are valid."""
        validate_uniform_random_matrix_properties(A_L, noise_range=noise_range)
        validate_uniform_random_matrix_properties(B_R, noise_range=noise_range)
        validate_permutation_matrix_properties(A_R, dim=0)
        validate_permutation_matrix_properties(B_L, dim=1)
        validate_matrix_product_properties(A_L, A_R, noise_range=noise_range)
        validate_matrix_product_properties(B_L, B_R, noise_range=noise_range)

    def test_generate_noise_metrices_basic(self, medium_generator, test_key):
        """Test the main generate_noise_metrices method."""
        A_L, A_R, B_L, B_R = medium_generator.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        assert A_L.shape == (128, 64)  # A_rows=128, noise_rank=64
        assert A_R.shape == (64, 256)  # noise_rank=64, common_dim=256
        assert B_L.shape == (256, 64)  # common_dim=256, noise_rank=64
        assert B_R.shape == (64, 128)  # noise_rank=64, B_cols=128

        # Validate using helper functions
        self.validate_noise_metrices(A_L, A_R, B_L, B_R, noise_range=128)

    def test_generate_noise_metrices_deterministic(self, test_key):
        """Test that noise generation is deterministic for the same key."""
        generator1 = NoiseGenerator(noise_rank=64, noise_range=128)
        generator2 = NoiseGenerator(noise_rank=64, noise_range=128)

        A_L1, A_R1, B_L1, B_R1 = generator1.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )
        A_L2, A_R2, B_L2, B_R2 = generator2.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        assert torch.equal(A_L1, A_L2)
        assert torch.equal(A_R1, A_R2)
        assert torch.equal(B_L1, B_L2)
        assert torch.equal(B_R1, B_R2)

    def test_generate_noise_metrices_different_keys(self, test_key, alt_test_key):
        """Test that different keys produce different noise matrices."""
        generator1 = NoiseGenerator(noise_rank=64, noise_range=128)
        generator2 = NoiseGenerator(noise_rank=64, noise_range=128)

        A_L1, A_R1, B_L1, B_R1 = generator1.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )
        A_L2, A_R2, B_L2, B_R2 = generator2.generate_noise_metrices(
            key_A=alt_test_key,
            key_B=alt_test_key,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        # At least one matrix should be different
        assert not (
            torch.equal(A_L1, A_L2)
            and torch.equal(A_R1, A_R2)
            and torch.equal(B_L1, B_L2)
            and torch.equal(B_R1, B_R2)
        )

    def test_generate_noise_metrices_value_ranges(self, large_generator, test_key):
        """Test that generated noise matrices have values within expected range."""
        A_L, A_R, B_L, B_R = large_generator.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=256,
            common_dim=384,
            B_cols=256,
        )

        self.validate_noise_metrices(A_L, A_R, B_L, B_R, noise_range=128)

    def test_rank_equals_matrix_dimension(self, medium_generator, test_key):
        """Test when noise_rank equals one of the matrix dimensions."""
        # Test when one dimension equals noise_rank - need dimensions >= noise_rank
        A_L, A_R, B_L, B_R = medium_generator.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=64,
            common_dim=128,
            B_cols=64,
        )

        assert A_L.shape == (64, 64)
        assert A_R.shape == (64, 128)
        assert B_L.shape == (128, 64)
        assert B_R.shape == (64, 64)

        self.validate_noise_metrices(A_L, A_R, B_L, B_R, noise_range=128)

    def test_large_matrices(self, large_generator, test_key):
        """Test with larger matrix dimensions."""
        A_L, A_R, B_L, B_R = large_generator.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=500,
            common_dim=750,
            B_cols=600,
        )

        assert A_L.shape == (500, 128)
        assert A_R.shape == (128, 750)
        assert B_L.shape == (750, 128)
        assert B_R.shape == (128, 600)

        self.validate_noise_metrices(A_L, A_R, B_L, B_R, noise_range=128)

    def test_asymmetric_dimensions(self, medium_generator, test_key):
        """Test with very asymmetric matrix dimensions."""
        A_L, A_R, B_L, B_R = medium_generator.generate_noise_metrices(
            key_A=test_key,
            key_B=test_key,
            A_rows=100,
            common_dim=1000,
            B_cols=2000,
        )

        assert A_L.shape == (100, 64)
        assert A_R.shape == (64, 1000)
        assert B_L.shape == (1000, 64)
        assert B_R.shape == (64, 2000)

        self.validate_noise_metrices(A_L, A_R, B_L, B_R, noise_range=128)


class TestNoiseGeneratorEdgeCases:
    """Test suite for edge cases and error conditions."""

    def test_dimension_validation_errors(self, small_generator, test_key):
        """Test that dimensions smaller than noise_rank raise errors."""
        with pytest.raises(
            ValueError,
            match="A_rows must be greater than or equal to noise_rank",
        ):
            small_generator.generate_noise_metrices(
                key_A=test_key,
                key_B=test_key,
                A_rows=16,
                common_dim=64,
                B_cols=64,
            )  # A_rows=16 < noise_rank=32

        with pytest.raises(
            ValueError,
            match="common_dim must be greater than or equal to noise_rank",
        ):
            small_generator.generate_noise_metrices(
                key_A=test_key,
                key_B=test_key,
                A_rows=64,
                common_dim=16,
                B_cols=64,
            )  # common_dim=16 < noise_rank=32

        with pytest.raises(
            ValueError,
            match="B_cols must be greater than or equal to noise_rank",
        ):
            small_generator.generate_noise_metrices(
                key_A=test_key,
                key_B=test_key,
                A_rows=64,
                common_dim=64,
                B_cols=16,
            )  # B_cols=16 < noise_rank=32

    def test_various_matrix_sizes(self, small_generator, test_key):
        """Test with various valid matrix sizes."""
        test_cases = [
            (32, 64, 32),  # Minimum valid sizes
            (50, 100, 75),  # Medium sizes
            (32, 32, 32),  # All equal to noise_rank
        ]

        for A_rows, common_dim, B_cols in test_cases:
            A_L, A_R, B_L, B_R = small_generator.generate_noise_metrices(
                key_A=test_key,
                key_B=test_key,
                A_rows=A_rows,
                common_dim=common_dim,
                B_cols=B_cols,
            )
            assert A_L.shape == (A_rows, 32)
            assert A_R.shape == (32, common_dim)
            assert B_L.shape == (common_dim, 32)
            assert B_R.shape == (32, B_cols)

import pytest
import torch
from miner_base.noise_generation import NoiseGenerator
from miner_base.noisy_gemm import POW_TARGET_EASIEST, POW_TARGET_HARDEST, NoisyGemm
from lattice_gateway.comm.dataclasses import CommitmentHash


@pytest.fixture
def test_commitment_hash() -> CommitmentHash:
    """Fixed test commitment hash for reproducible results."""
    return CommitmentHash(
        noise_seed_A=b"test_key_A_123456789012345678901",
        noise_seed_B=b"test_key_B_123456789012345678901",
    )


@pytest.fixture
def noise_generator() -> NoiseGenerator:
    """Create a NoiseGenerator instance for testing."""
    return NoiseGenerator(noise_rank=64, noise_range=128)


@pytest.fixture
def noisy_gemm_instance() -> NoisyGemm:
    """Create a NoisyGemm instance for testing."""
    # Use smaller noise_rank to work with test matrix sizes
    return NoisyGemm(noise_range=128, noise_rank=64, matmul_tile_h=64, matmul_tile_w=64)


class TestNoisyGemmCore:
    """Test suite for core NoisyGemm functionality."""

    @pytest.mark.parametrize(
        "m,k,n,expect_block",
        [
            (64, 128, 64, False),
            (128, 256, 128, True),
            (256, 128, 256, False),
            (64, 256, 128, True),
        ],
    )
    @pytest.mark.parametrize(
        "matmul_tile_h,matmul_tile_w",
        [(32, 46), (48, 64), (64, 64)],
    )
    def test_various_matrix_sizes(
        self,
        noise_generator,
        test_commitment_hash,
        m,
        k,
        n,
        expect_block,
        matmul_tile_h,
        matmul_tile_w,
    ):
        """Test noisy_gemm with various matrix dimensions."""
        A = torch.randint(-64, 64, (m, k), dtype=torch.int8)  # int7 range [-64, 63]
        B = torch.randint(-64, 64, (k, n), dtype=torch.int8)
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=m,
            common_dim=k,
            B_cols=n,
        )

        gemm_instance = NoisyGemm(
            noise_range=128,
            noise_rank=64,
            matmul_tile_h=matmul_tile_h,
            matmul_tile_w=matmul_tile_w,
        )

        # Use easiest target when expecting block, hardest when not
        pow_target = POW_TARGET_EASIEST if expect_block else POW_TARGET_HARDEST

        result, found_block = gemm_instance.noisy_gemm(
            A,
            B,
            E_AL,
            E_AR,
            E_BL,
            E_BR,
            commitment_hash=test_commitment_hash,
            pow_target=pow_target,
        )
        expected = torch.matmul(A.to(torch.int32), B.to(torch.int32))

        assert result.shape == expected.shape
        assert result.dtype == expected.dtype
        assert torch.equal(result, expected)
        assert found_block is expect_block


class TestNoisyGemmValidation:
    """Test input validation and error handling."""

    def test_invalid_dtype_rejection_noisy_gemm(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test that invalid dtypes are rejected in noisy_gemm."""
        A = torch.randn(64, 128)  # float32 instead of int8
        B = torch.randint(-64, 64, (128, 64), dtype=torch.int8)
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=64,
            common_dim=128,
            B_cols=64,
        )

        with pytest.raises(ValueError, match="must be int8"):
            noisy_gemm_instance.noisy_gemm(
                A,
                B,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

    def test_invalid_dtype_rejection_noise_A(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test that invalid dtypes are rejected in noise_A."""
        A = torch.randn(128, 256)  # float32 instead of int8
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        with pytest.raises(ValueError, match="must be int8"):
            noisy_gemm_instance.noise_A(A, E_AL, E_AR, E_BL)

    def test_invalid_dtype_rejection_noise_B(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test that invalid dtypes are rejected in noise_B."""
        B = torch.randn(256, 128)  # float32 instead of int8
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        with pytest.raises(ValueError, match="must be int8"):
            noisy_gemm_instance.noise_B(B, E_AR, E_BL, E_BR)

    def test_invalid_dtype_rejection_gemm(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test that invalid dtypes are rejected in gemm."""
        m, k, n = 128, 256, 128
        A = torch.randint(-64, 64, (m, k), dtype=torch.int8)
        B = torch.randint(-64, 64, (k, n), dtype=torch.int8)
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=m,
            common_dim=k,
            B_cols=n,
        )

        A_noised, A_E_BL = noisy_gemm_instance.noise_A(A, E_AL, E_AR, E_BL)
        B_noised, EAR_BpEB = noisy_gemm_instance.noise_B(B, E_AR, E_BL, E_BR)

        A_noised_wrong = A_noised.to(torch.float32)
        with pytest.raises(ValueError, match="must be int8"):
            noisy_gemm_instance.gemm(
                A_noised_wrong,
                B_noised,
                E_AL,
                E_BR,
                A_E_BL,
                EAR_BpEB,
                pow_key=test_commitment_hash.noise_seed_A,
                pow_target=POW_TARGET_HARDEST,
            )

    def test_range_validation_noisy_gemm(
        self, noisy_gemm_instance, noise_generator, test_commitment_hash
    ):
        """Test that values outside range are rejected in noisy_gemm."""
        m, k, n = 64, 128, 64
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=m,
            common_dim=k,
            B_cols=n,
        )

        # Test A with values outside int7 range
        A_invalid = torch.full((m, k), -65, dtype=torch.int8)  # Below -64
        B_valid = torch.randint(-64, 64, (k, n), dtype=torch.int8)

        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noisy_gemm(
                A_invalid,
                B_valid,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

        # Test A with values above 63
        A_invalid = torch.full((m, k), 64, dtype=torch.int8)  # Above 63
        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noisy_gemm(
                A_invalid,
                B_valid,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

        # Test B with values outside int7 range
        A_valid = torch.randint(-64, 64, (m, k), dtype=torch.int8)
        B_invalid = torch.full((k, n), -65, dtype=torch.int8)  # Below -64

        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noisy_gemm(
                A_valid,
                B_invalid,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

        # Test B with values above 63
        B_invalid = torch.full((k, n), 64, dtype=torch.int8)  # Above 63
        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noisy_gemm(
                A_valid,
                B_invalid,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

    def test_range_validation_noise_A(
        self, noisy_gemm_instance, noise_generator, test_commitment_hash
    ):
        """Test that values outside range are rejected in noise_A."""
        m, k = 128, 256
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=m,
            common_dim=k,
            B_cols=128,
        )

        # Test with values below -64
        A_invalid = torch.full((m, k), -65, dtype=torch.int8)
        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noise_A(A_invalid, E_AL, E_AR, E_BL)

        # Test with values above 63
        A_invalid = torch.full((m, k), 64, dtype=torch.int8)
        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noise_A(A_invalid, E_AL, E_AR, E_BL)

    def test_range_validation_noise_B(
        self, noisy_gemm_instance, noise_generator, test_commitment_hash
    ):
        """Test that values outside range are rejected in noise_B."""
        k, n = 256, 128
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=128,
            common_dim=k,
            B_cols=n,
        )

        # Test with values below -64
        B_invalid = torch.full((k, n), -65, dtype=torch.int8)
        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noise_B(B_invalid, E_AR, E_BL, E_BR)

        # Test with values above 63
        B_invalid = torch.full((k, n), 64, dtype=torch.int8)
        with pytest.raises(ValueError, match="tensor must be in the range"):
            noisy_gemm_instance.noise_B(B_invalid, E_AR, E_BL, E_BR)

    def test_dimension_mismatch_errors(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test various dimension mismatch scenarios."""
        A = torch.randint(-64, 64, (64, 128), dtype=torch.int8)
        B = torch.randint(-64, 64, (128, 64), dtype=torch.int8)
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=64,
            common_dim=128,
            B_cols=64,
        )

        # Test mismatched A dimensions
        A_wrong = torch.randint(-64, 64, (32, 128), dtype=torch.int8)  # Wrong m
        with pytest.raises(ValueError):
            noisy_gemm_instance.noisy_gemm(
                A_wrong,
                B,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

        # Test mismatched B dimensions
        B_wrong = torch.randint(-64, 64, (128, 32), dtype=torch.int8)  # Wrong n
        with pytest.raises(ValueError):
            noisy_gemm_instance.noisy_gemm(
                A,
                B_wrong,
                E_AL,
                E_AR,
                E_BL,
                E_BR,
                commitment_hash=test_commitment_hash,
            )

    def test_range_initialization_validation(self):
        """Test NoisyGemm range parameter validation."""
        with pytest.raises(ValueError, match="range must fit in uint7"):
            NoisyGemm(noise_range=256)

    def test_noise_A_dimension_validation(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test dimension validation in noise_A method."""
        A = torch.randint(-64, 64, (128, 256), dtype=torch.int8)
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        # Test with wrong E_AL dimensions
        E_AL_wrong = torch.randint(0, 127, (64, 64), dtype=torch.int8)  # Wrong m dimension
        with pytest.raises(ValueError):
            noisy_gemm_instance.noise_A(A, E_AL_wrong, E_AR, E_BL)

    def test_noise_B_dimension_validation(
        self,
        noisy_gemm_instance,
        noise_generator,
        test_commitment_hash,
    ):
        """Test dimension validation in noise_B method."""
        B = torch.randint(-64, 64, (256, 128), dtype=torch.int8)
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=128,
            common_dim=256,
            B_cols=128,
        )

        # Test with wrong E_BR dimensions
        E_BR_wrong = torch.randint(0, 127, (64, 64), dtype=torch.int8)  # Wrong n dimension
        with pytest.raises(ValueError):
            noisy_gemm_instance.noise_B(B, E_AR, E_BL, E_BR_wrong)

    def test_matrix_dimension_vs_rank_validation(self, test_commitment_hash):
        """Test that matrix dimensions must be >= noise_rank."""
        noise_rank = 64
        noisy_gemm_instance = NoisyGemm(noise_range=128, noise_rank=noise_rank)

        def test_case(m, k, n):
            A = torch.randint(-64, 64, (m, k), dtype=torch.int8)
            B = torch.randint(-64, 64, (k, n), dtype=torch.int8)
            E_AL = torch.randint(0, 127, (m, noise_rank), dtype=torch.int8)
            E_AR = torch.randint(0, 127, (noise_rank, k), dtype=torch.int8)
            E_BL = torch.randint(0, 127, (k, noise_rank), dtype=torch.int8)
            E_BR = torch.randint(0, 127, (noise_rank, n), dtype=torch.int8)

            with pytest.raises(
                ValueError,
                match="expected A.shape\\[0\\] and A.shape\\[1\\] and B.shape\\[1\\] to be greater than matmul_tile_h and noise_rank and matmul_tile_w",
            ):
                noisy_gemm_instance.noisy_gemm(
                    A,
                    B,
                    E_AL,
                    E_AR,
                    E_BL,
                    E_BR,
                    commitment_hash=test_commitment_hash,
                )

        test_case(32, 128, 128)  # m < noise_rank
        test_case(128, 32, 128)  # k < noise_rank
        test_case(128, 128, 32)  # n < noise_rank


class TestNoisyGemmEdgeCases:
    """Test edge cases and boundary conditions."""

    def _test_noisy_gemm(
        self,
        A: torch.Tensor,
        B: torch.Tensor,
        noise_generator: NoiseGenerator,
        noisy_gemm: NoisyGemm,
        test_commitment_hash: CommitmentHash,
    ):
        """Test noisy_gemm with a given matrix pair."""
        assert A.shape[1] == B.shape[0], "A and B must have compatible dimensions"
        E_AL, E_AR, E_BL, E_BR = noise_generator.generate_noise_metrices(
            key_A=test_commitment_hash.noise_seed_A,
            key_B=test_commitment_hash.noise_seed_B,
            A_rows=A.shape[0],
            common_dim=A.shape[1],
            B_cols=B.shape[1],
        )

        # Use POW_TARGET_HARDEST so no block is found
        result, found_block = noisy_gemm.noisy_gemm(
            A,
            B,
            E_AL,
            E_AR,
            E_BL,
            E_BR,
            commitment_hash=test_commitment_hash,
            pow_target=POW_TARGET_HARDEST,
        )
        expected = torch.matmul(A.to(torch.int32), B.to(torch.int32))

        assert result.shape == expected.shape
        assert result.dtype == expected.dtype
        assert torch.equal(result, expected)
        assert found_block is False

    def test_minimum_matrix_sizes(self, noise_generator, noisy_gemm_instance, test_commitment_hash):
        """Test with minimum viable matrix sizes."""

        # Minimum size that works with noise_rank=64
        m, k, n = 64, 64, 64
        A = torch.randint(-64, 64, (m, k), dtype=torch.int8)
        B = torch.randint(-64, 64, (k, n), dtype=torch.int8)

        self._test_noisy_gemm(A, B, noise_generator, noisy_gemm_instance, test_commitment_hash)

    def test_zero_matrices(self, noise_generator, noisy_gemm_instance, test_commitment_hash):
        """Test behavior with zero matrices."""
        m, k, n = 128, 256, 128
        A = torch.zeros((m, k), dtype=torch.int8)
        B = torch.zeros((k, n), dtype=torch.int8)
        self._test_noisy_gemm(A, B, noise_generator, noisy_gemm_instance, test_commitment_hash)

    def test_identity_like_matrices(
        self, noise_generator, noisy_gemm_instance, test_commitment_hash
    ):
        """Test with identity-like patterns."""
        m, k, n = 128, 128, 128
        # Create identity-like matrix (but int8)
        A = torch.eye(m, k, dtype=torch.int8)
        B = torch.eye(k, n, dtype=torch.int8)
        self._test_noisy_gemm(A, B, noise_generator, noisy_gemm_instance, test_commitment_hash)

    def test_different_ranges(self, test_commitment_hash):
        """Test with different range values."""
        generator = NoiseGenerator(noise_rank=64, noise_range=64)
        gemm = NoisyGemm(noise_range=64, noise_rank=64)

        m, k, n = 128, 256, 128
        # Use smaller range for this test to match the noise_range=64
        A = torch.randint(-32, 32, (m, k), dtype=torch.int8)  # int6 range [-32, 31]
        B = torch.randint(-32, 32, (k, n), dtype=torch.int8)
        self._test_noisy_gemm(A, B, generator, gemm, test_commitment_hash)

    def test_int7_boundary_values(self, noise_generator, noisy_gemm_instance, test_commitment_hash):
        """Test with boundary values of int7 range."""
        m, k, n = 128, 256, 128
        # Test with exact boundary values
        A = torch.full((m, k), -64, dtype=torch.int8)  # Min int7 value
        B = torch.full((k, n), 63, dtype=torch.int8)  # Max int7 value
        self._test_noisy_gemm(A, B, noise_generator, noisy_gemm_instance, test_commitment_hash)

    def test_minimum_tile_size_validation(self):
        """Test that matmul tile size must be greater than hash tile size."""
        noise_rank = 8
        with pytest.raises(
            ValueError,
            match=f"matmul_tile_h={noise_rank} and matmul_tile_w={noise_rank} must be greater than or equal to hash_tile_h=16 and hash_tile_w=16",
        ):
            NoisyGemm(
                noise_range=128,
                noise_rank=noise_rank,
                matmul_tile_h=noise_rank,
                matmul_tile_w=noise_rank,
            )


class TestTiledMatmul:
    """Test suite for _tiled_matmul functionality."""

    @pytest.mark.parametrize(
        "m,k,n,noise_rank",
        [
            # Basic cases where dimensions are multiples of noise_rank
            (64, 64, 64, 16),
            (64, 128, 64, 16),
            (128, 256, 128, 32),
            # Cases where dimensions are not exact multiples of noise_rank but still result in tiles >= 16x16
            (48, 80, 48, 16),  # Last tile will be 16x16
            (96, 144, 96, 32),  # Last tile will be 32x16
            (80, 96, 64, 16),  # Last tile will be 16x16
            # Cases where noise_rank equals matrix dimensions
            (16, 16, 16, 16),
            (32, 32, 32, 32),
            (64, 64, 64, 64),
        ],
    )
    def test_tiled_matmul_vs_regular_matmul(self, m, k, n, noise_rank, test_commitment_hash):
        """Test that _tiled_matmul produces identical results to regular matmul."""
        # Create NoisyGemm instance with specified noise_rank
        noisy_gemm = NoisyGemm(noise_range=128, noise_rank=noise_rank)

        # Generate random test matrices with int8 values (as expected by _tiled_matmul)
        A = torch.randint(-64, 64, (m, k), dtype=torch.int8)
        B = torch.randint(-64, 64, (k, n), dtype=torch.int8)

        # Compute results using both methods
        result_tiled, found_block = noisy_gemm._tiled_matmul(
            A, B, pow_key=test_commitment_hash.noise_seed_A, pow_target=POW_TARGET_HARDEST
        )
        result_regular = torch.matmul(A.to(torch.int32), B.to(torch.int32))

        # Verify results are identical
        assert result_tiled.shape == result_regular.shape
        assert result_tiled.dtype == result_regular.dtype
        assert torch.equal(result_tiled, result_regular)
        # With POW_TARGET_HARDEST, we should never find a block
        assert found_block is False

    def test_tiled_matmul_different_dtypes(self, test_commitment_hash):
        """Test _tiled_matmul with different dtypes."""
        noise_rank = 16
        m, k, n = 64, 128, 64
        noisy_gemm = NoisyGemm(noise_range=128, noise_rank=noise_rank)

        # Test with int8
        A_int8 = torch.randint(-64, 64, (m, k), dtype=torch.int8)
        B_int8 = torch.randint(-64, 64, (k, n), dtype=torch.int8)

        result_tiled, found_block = noisy_gemm._tiled_matmul(
            A_int8, B_int8, pow_key=test_commitment_hash.noise_seed_A, pow_target=POW_TARGET_HARDEST
        )
        result_regular = torch.matmul(A_int8.to(torch.int32), B_int8.to(torch.int32))

        assert torch.equal(result_tiled, result_regular)
        assert found_block is False

    def test_tiled_matmul_dimension_mismatch(self, test_commitment_hash):
        """Test that _tiled_matmul handles dimension mismatches correctly."""
        noise_rank = 16
        noisy_gemm = NoisyGemm(noise_range=128, noise_rank=noise_rank)

        A = torch.randint(-64, 64, (32, 64), dtype=torch.int8)
        B = torch.randint(-64, 64, (128, 32), dtype=torch.int8)  # Wrong dimensions

        # This should raise an assertion error due to dimension mismatch
        with pytest.raises(AssertionError):
            noisy_gemm._tiled_matmul(
                A, B, pow_key=test_commitment_hash.noise_seed_A, pow_target=POW_TARGET_HARDEST
            )

    def test_tiled_matmul_zero_matrices(self, test_commitment_hash):
        """Test _tiled_matmul with zero matrices."""
        noise_rank = 16
        m, k, n = 64, 128, 64
        noisy_gemm = NoisyGemm(noise_range=128, noise_rank=noise_rank)

        A_zero = torch.zeros((m, k), dtype=torch.int8)
        B_zero = torch.zeros((k, n), dtype=torch.int8)

        result_tiled, found_block = noisy_gemm._tiled_matmul(
            A_zero, B_zero, pow_key=test_commitment_hash.noise_seed_A, pow_target=POW_TARGET_HARDEST
        )
        result_regular = torch.matmul(A_zero.to(torch.int32), B_zero.to(torch.int32))

        assert torch.equal(result_tiled, result_regular)
        assert torch.all(result_tiled == 0)
        assert found_block is False

    def test_tiled_matmul_identity_like_matrices(self, test_commitment_hash):
        """Test _tiled_matmul with identity-like matrices."""
        noise_rank = 16
        size = 64
        noisy_gemm = NoisyGemm(noise_range=128, noise_rank=noise_rank)

        # Create identity-like matrices (but with int8 dtype)
        A = torch.eye(size, dtype=torch.int8)
        B = torch.eye(size, dtype=torch.int8) * 2  # Scaled identity

        result_tiled, found_block = noisy_gemm._tiled_matmul(
            A, B, pow_key=test_commitment_hash.noise_seed_A, pow_target=POW_TARGET_HARDEST
        )
        result_regular = torch.matmul(A.to(torch.int32), B.to(torch.int32))

        assert torch.equal(result_tiled, result_regular)
        assert found_block is False

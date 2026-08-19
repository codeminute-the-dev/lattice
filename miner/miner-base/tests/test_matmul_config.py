"""Tests for MatmulConfig and pattern factories."""

import pytest
from miner_base.settings import MinerSettings
from lattice_gateway.comm.mining_configuration import PeriodicPattern

_settings = MinerSettings()

ALL_PATTERNS = [
    PeriodicPattern.from_list(_settings.rows_pattern),
    PeriodicPattern.from_list(_settings.cols_pattern),
]


class TestPatternValidity:
    """Test that all patterns in the codebase are valid."""

    @pytest.mark.parametrize("pattern", ALL_PATTERNS)
    def test_all_codebase_patterns_are_valid(self, pattern: PeriodicPattern):
        """Test that all patterns in the codebase are valid."""
        assert pattern.is_valid(), f"Pattern with shape {pattern.shape} is not valid"

    @pytest.mark.parametrize("pattern", ALL_PATTERNS)
    def test_offset_partitions_space(self, pattern: PeriodicPattern):
        """Test that valid offsets + pattern elements create a perfect partition."""
        # Ensure max_offset is a multiple of period for complete coverage
        num_periods = max(4, 1000 // pattern.period + 1)
        max_offset = num_periods * pattern.period
        pattern_list = pattern.to_list()

        valid_offsets = [off for off in range(max_offset) if pattern.offset_is_valid(off)]

        all_indices = []
        for offset in valid_offsets:
            all_indices.extend(offset + p for p in pattern_list)

        assert len(all_indices) == len(set(all_indices)), (
            f"Duplicate indices found, {pattern.to_list()=}"
        )

        max_index = max(all_indices)
        assert sorted(all_indices) == list(range(max_index + 1)), (
            f"Indices don't cover [0, {max_index + 1}), {pattern.to_list()=}"
        )

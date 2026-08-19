"""
Test configuration loading and validation.
"""

import os
from unittest.mock import patch

import pytest
from lattice_gateway.config import (
    MinerRpcConfig,
    LatticeConfig,
    LatticeGatewayConfig,
    load_config,
)
from pydantic import ValidationError


class TestLatticeConfig:
    """Test LatticeConfig."""

    def test_lattice_config_defaults(self, mining_address):
        """Test LatticeConfig with default values."""
        config = LatticeConfig(mining_address=mining_address)

        assert config.rpc_url == "http://0.0.0.0:44107"
        assert config.rpc_user == "user"
        assert config.rpc_password == "pass"
        assert config.refresh_interval_seconds == 1
        assert config.mining_address == mining_address

    def test_lattice_config_missing_mining_address(self):
        """Test LatticeConfig with missing mining address."""
        with pytest.raises(ValidationError, match="mining_address"):
            LatticeConfig()

    def test_lattice_config_custom_values(self, mining_address):
        """Test creating LatticeConfig with custom values."""
        config = LatticeConfig(
            rpc_url="https://127.0.0.1:18334",
            rpc_user="test_user",
            rpc_password="test_password",
            refresh_interval_seconds=5,
            mining_address=mining_address,
        )

        assert config.rpc_url == "https://127.0.0.1:18334"
        assert config.rpc_user == "test_user"
        assert config.rpc_password == "test_password"
        assert config.refresh_interval_seconds == 5
        assert config.mining_address == mining_address

    def test_lattice_config_env_override(self, mining_address):
        """Test LatticeConfig with environment variable overrides."""
        with patch.dict(
            os.environ,
            {
                "LATTICED_RPC_URL": "https://production:8334",
                "LATTICED_RPC_USER": "prod_user",
                "LATTICED_RPC_PASSWORD": "prod_pass",
                "LATTICED_MINING_ADDRESS": mining_address,
            },
        ):
            config = LatticeConfig()

            assert config.rpc_url == "https://production:8334"
            assert config.rpc_user == "prod_user"
            assert config.rpc_password == "prod_pass"
            assert config.mining_address == mining_address


class TestMinerRpcConfig:
    """Test MinerRpcConfig."""

    def test_miner_rpc_config_defaults(self):
        """Test MinerRpcConfig with default values."""
        config = MinerRpcConfig()

        assert config.transport == "uds"
        assert config.socket_path == "/tmp/latticegw.sock"
        assert config.port == 8337
        assert config.host == "localhost"

    def test_miner_rpc_config_uds_transport(self):
        """Test MinerRpcConfig with UDS transport."""
        config = MinerRpcConfig(
            transport="uds",
            socket_path="/tmp/test.sock",
        )

        assert config.transport == "uds"
        assert config.socket_path == "/tmp/test.sock"

    def test_miner_rpc_config_tcp_transport(self):
        """Test MinerRpcConfig with TCP transport."""
        config = MinerRpcConfig(
            transport="tcp",
            port=9000,
            host="192.168.1.1",
        )

        assert config.transport == "tcp"
        assert config.port == 9000
        assert config.host == "192.168.1.1"

    def test_miner_rpc_config_invalid_transport(self):
        """Test MinerRpcConfig with invalid transport."""
        with pytest.raises(ValidationError, match="Input should be 'uds' or 'tcp'"):
            MinerRpcConfig(transport="invalid")

    def test_miner_rpc_config_tcp_without_port(self):
        """Test MinerRpcConfig TCP transport without port."""
        with pytest.raises(ValueError, match="TCP transport requires port"):
            MinerRpcConfig(transport="tcp", port=None)

    def test_miner_rpc_config_uds_without_socket_path(self):
        """Test MinerRpcConfig UDS transport without socket_path."""
        with pytest.raises(ValueError, match="UDS transport requires socket_path"):
            MinerRpcConfig(transport="uds", socket_path=None)

    def test_miner_rpc_config_env_override(self):
        """Test MinerRpcConfig with environment variable overrides."""
        with patch.dict(
            os.environ,
            {
                "MINER_RPC_TRANSPORT": "tcp",
                "MINER_RPC_PORT": "9000",
                "MINER_RPC_HOST": "192.168.1.100",
            },
        ):
            config = MinerRpcConfig()

            assert config.transport == "tcp"
            assert config.port == 9000
            assert config.host == "192.168.1.100"


class TestLoadConfig:
    """Test load_config function."""

    def test_load_config_defaults(self, mining_address):
        """Test loading configuration with all defaults."""
        with patch.dict(
            os.environ,
            {"LATTICED_MINING_ADDRESS": mining_address},
        ):
            config = load_config()

        assert isinstance(config, LatticeGatewayConfig)
        assert isinstance(config.lattice, LatticeConfig)
        assert isinstance(config.miner_rpc, MinerRpcConfig)

        # Verify some defaults
        assert config.lattice.rpc_url == "http://0.0.0.0:44107"
        assert config.miner_rpc.transport == "uds"

    def test_load_config_with_env_overrides(self, mining_address):
        """Test loading configuration with environment variable overrides."""
        with patch.dict(
            os.environ,
            {
                "LATTICED_RPC_URL": "https://custom:8334",
                "LATTICED_RPC_USER": "custom_user",
                "LATTICED_MINING_ADDRESS": mining_address,
            },
        ):
            config = load_config()

            assert config.lattice.rpc_url == "https://custom:8334"
            assert config.lattice.rpc_user == "custom_user"
            assert config.lattice.mining_address == mining_address

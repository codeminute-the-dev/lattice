from typing import Literal

from pydantic import BaseModel
from pydantic_settings import BaseSettings, SettingsConfigDict


class LatticeConfig(BaseSettings):
    """Lattice node connection configuration.

    Environment variables override defaults:
    - LATTICED_RPC_URL
    - LATTICED_RPC_USER
    - LATTICED_RPC_PASSWORD
    - LATTICED_REFRESH_INTERVAL_SECONDS
    - LATTICED_MINING_ADDRESS
    """

    model_config = SettingsConfigDict(env_prefix="LATTICED_")

    rpc_url: str = "http://0.0.0.0:44107"
    rpc_user: str = "user"
    rpc_password: str = "pass"
    refresh_interval_seconds: int = 1
    mining_address: str


class MinerRpcConfig(BaseSettings):
    """Miner RPC server configuration.

    Environment variables override defaults:
    - MINER_RPC_TRANSPORT
    - MINER_RPC_SOCKET_PATH
    - MINER_RPC_PORT
    - MINER_RPC_HOST
    """

    model_config = SettingsConfigDict(env_prefix="MINER_RPC_")

    transport: Literal["uds", "tcp"] = "uds"
    socket_path: str | None = "/tmp/latticegw.sock"
    port: int | None = 8337
    host: str | None = "localhost"

    def model_post_init(self, __context) -> None:
        """Validate configuration after initialization."""
        if self.transport == "tcp" and not self.port:
            raise ValueError("TCP transport requires port to be specified")
        elif self.transport == "uds" and not self.socket_path:
            raise ValueError("UDS transport requires socket_path")


class LatticeGatewayConfig(BaseModel):
    """Complete LatticeGateway configuration.

    All nested configs support environment variable overrides.
    See individual config classes for available environment variables.
    """

    lattice: LatticeConfig
    miner_rpc: MinerRpcConfig


def load_config() -> LatticeGatewayConfig:
    """Load configuration from environment variables with defaults.

    Priority (highest to lowest):
    1. Environment variables (e.g., LATTICED_RPC_URL)
    2. Default values defined in the config classes

    Returns:
        LatticeGatewayConfig: Fully configured gateway settings
    """
    return LatticeGatewayConfig(
        lattice=LatticeConfig(),
        miner_rpc=MinerRpcConfig(),
    )

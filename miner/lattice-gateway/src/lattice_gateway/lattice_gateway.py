"""
Main application class that orchestrates all components.
"""

import os
import tempfile

from miner_utils import get_logger

from lattice_gateway.config import MinerRpcConfig, load_config
from lattice_gateway.miner_rpc.server import MinerRpcServer
from lattice_gateway.lattice_client import LatticeNodeClient
from lattice_gateway.scheduler import TemplateScheduler
from lattice_gateway.submission_service import SubmissionService
from lattice_gateway.work_cache import WorkCache


class LatticeGateway:
    """
    Main application class that orchestrates all components.
    """

    def __init__(
        self,
        use_temp_socket: bool = False,
        debug_mode: bool = False,
    ):
        """
        Initialize the LatticeGateway.

        Args:
            use_temp_socket: If True, use a temporary file for the UDS socket path.
                           Useful for testing to avoid socket conflicts.
            debug_mode: Enable debug mode logging.
        """
        # Load configuration from environment variables and defaults
        self.config = load_config()

        # Set up logging
        self.logger = get_logger(__name__)

        # Track temp socket for cleanup
        self._temp_socket_path: str | None = None

        # Override socket path for testing isolation
        if use_temp_socket:
            self._temp_socket_path = tempfile.mktemp(suffix=".sock")
            self.config.miner_rpc = MinerRpcConfig(
                transport="uds",
                socket_path=self._temp_socket_path,
            )

        # Initialize components (but don't start them yet)
        self.work_cache = WorkCache()
        self.lattice_client = LatticeNodeClient(self.config.lattice)
        self.submission_service = SubmissionService(self.lattice_client, debug_mode=debug_mode)
        self.miner_rpc = MinerRpcServer(
            self.work_cache,
            self.submission_service,
            self.config.miner_rpc,
        )
        self.scheduler = TemplateScheduler(self.lattice_client, self.work_cache, self.config.lattice)

        self.running = False
        self.logger.info("LatticeGateway initialized")

    async def start(self):
        """Start all components of the gateway."""
        if self.running:
            self.logger.warning("LatticeGateway is already running")
            return

        self.logger.info("Starting LatticeGateway")

        # Start Lattice client
        await self.lattice_client.__aenter__()

        # Start the scheduler (which will immediately fetch a block template)
        await self.scheduler.start()

        # Start the Miner RPC server
        await self.miner_rpc.start()

        self.running = True

        self.logger.info("LatticeGateway started successfully")

    async def stop(self):
        """Gracefully stop all components."""
        if not self.running:
            self.logger.warning("LatticeGateway is not running")
            return

        self.logger.info("Stopping LatticeGateway")
        self.running = False

        # Stop components in reverse order
        await self.miner_rpc.stop()
        await self.scheduler.stop()
        await self.lattice_client.__aexit__(None, None, None)

        # Clean up temp socket if we created one
        if self._temp_socket_path and os.path.exists(self._temp_socket_path):
            os.unlink(self._temp_socket_path)

        self.logger.info("LatticeGateway stopped")

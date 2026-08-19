#! /usr/bin/env python3

"""
CLI for LatticeGateway.
"""

import argparse
import asyncio
import os

from miner_utils import get_logger

from lattice_gateway.lattice_gateway import LatticeGateway

logger = get_logger(__name__)


def parse_args():
    """Parse command line arguments."""
    parser = argparse.ArgumentParser(description="LatticeGateway - Lattice Mining Proxy")

    parser.add_argument(
        "command",
        choices=["start", "stop", "status", "version"],
        help="Command to execute",
    )

    # Debug mode (can also be set via MINER_DEBUG env var)
    parser.add_argument(
        "--debug",
        action="store_true",
        default=os.environ.get("MINER_DEBUG", "").lower() in ("1", "true", "yes"),
        help="Enable debug mode (default: from MINER_DEBUG env var)",
    )

    return parser.parse_args()


async def start_gateway(debug: bool = False):
    """Start the LatticeGateway service."""
    logger.debug("Starting LatticeGateway...")

    gateway = LatticeGateway(debug_mode=debug)
    await gateway.start()

    print("LatticeGateway is running. Press Ctrl+C to stop.")

    # Keep the service running until interrupted
    try:
        while True:
            await asyncio.sleep(1)
    except KeyboardInterrupt:
        print("Shutting down...")
    finally:
        await gateway.stop()


def stop_gateway():
    """Stop a running LatticeGateway service."""
    # In a production implementation, this would find and signal the running process
    # For now, just print instructions
    print("To stop LatticeGateway, press Ctrl+C in the terminal where it's running.")
    print("For a system service, use: sudo systemctl stop latticegw")


def show_status():
    """Show the status of the LatticeGateway service."""
    # In a production implementation, this would check if the process is running
    # For now, just print instructions
    print("To check LatticeGateway status, use:")
    print("  - For a system service: sudo systemctl status latticegw")
    print("  - For metrics: curl http://127.0.0.1:9109/metrics")


def show_version():
    """Show the version of LatticeGateway."""
    # This would normally read from a version file or package
    print("LatticeGateway v0.1.0")


def main():
    """Main entry point."""
    args = parse_args()

    if args.command == "start":
        asyncio.run(start_gateway(args.debug))
    elif args.command == "stop":
        stop_gateway()
    elif args.command == "status":
        show_status()
    elif args.command == "version":
        show_version()


if __name__ == "__main__":
    main()

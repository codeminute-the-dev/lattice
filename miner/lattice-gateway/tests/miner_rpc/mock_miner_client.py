import asyncio
import json


class MockMinerClient:
    """Mock miner client for integration testing."""

    def __init__(
        self,
        transport="tcp",
        host="127.0.0.1",
        port=18446,
        socket_path=None,
    ):
        self.transport = transport
        self.host = host
        self.port = port
        self.socket_path = socket_path
        self.request_id = 1
        self._reader = None
        self._writer = None

    async def __aenter__(self):
        await self.connect()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()

    async def connect(self):
        """Establish connection to the server."""
        if self.transport == "tcp":
            self._reader, self._writer = await asyncio.open_connection(self.host, self.port)
        else:  # UDS
            self._reader, self._writer = await asyncio.open_unix_connection(self.socket_path)

    async def close(self):
        """Close the connection."""
        if self._writer:
            self._writer.close()
            await self._writer.wait_closed()
            self._writer = None
            self._reader = None

    async def send_request(self, method, params=None):
        """Send a JSON-RPC request."""
        if params is None:
            params = {}

        request_data = {
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
            "id": self.request_id,
        }
        self.request_id += 1

        # Convert to line-delimited JSON
        request_line = json.dumps(request_data) + "\n"

        try:
            # Send request
            self._writer.write(request_line.encode())
            await self._writer.drain()

            # Read response
            response_line = await self._reader.readline()
            if not response_line:
                return 500, {"error": "Connection closed"}

            # Parse response
            response = json.loads(response_line.decode().strip())

            # Return status and response (compatible with old format)
            # JSON-RPC always returns 200 for valid protocol responses
            return 200, response

        except Exception as e:
            return 500, {"error": str(e)}

    async def get_mining_info(self):
        """Send getMiningInfo request."""
        return await self.send_request("getMiningInfo")

    async def submit_plain_proof(self, **kwargs):
        """Send submitPlainProof request."""
        # Pass the parameters directly as they should already be in the correct format
        return await self.send_request("submitPlainProof", kwargs)

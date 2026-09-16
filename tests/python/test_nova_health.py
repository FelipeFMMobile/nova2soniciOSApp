import asyncio
import unittest
import urllib.request
from unittest.mock import patch

from websockets.asyncio.server import serve
from nova_bridge.server import handle, health_response


class NovaHealthTests(unittest.IsolatedAsyncioTestCase):
    async def test_health_never_opens_a_bedrock_session(self):
        with patch("nova_bridge.server.NovaSession", side_effect=AssertionError("No inference during readiness")) as model:
            async with serve(handle, "127.0.0.1", 0, process_request=health_response) as server:
                port = server.sockets[0].getsockname()[1]

                def probe():
                    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
                    with opener.open(f"http://127.0.0.1:{port}/healthz", timeout=2) as response:
                        return response.status, response.read()

                self.assertEqual(await asyncio.to_thread(probe), (200, b"ok\n"))
                model.assert_not_called()

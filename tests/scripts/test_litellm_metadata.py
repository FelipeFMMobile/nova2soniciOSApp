import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("gate", ROOT / "scripts/check_litellm_metadata.py")
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)


class MetadataGateTests(unittest.TestCase):
    def inspect(self, forwarded):
        source = f"""
class Client:
    async def call_tool(self, call_tool_request_params):
        async def _call_tool_operation(session: ClientSession):
            return await session.call_tool(name=call_tool_request_params.name,
                arguments=call_tool_request_params.arguments{forwarded})
"""
        with tempfile.TemporaryDirectory() as directory:
            wheel = Path(directory) / "fixture.whl"
            with zipfile.ZipFile(wheel, "w") as archive:
                archive.writestr(gate.CLIENT, source)
                archive.writestr("litellm-fixture.dist-info/METADATA", "Version: fixture\n")
            return gate.inspect_wheel(wheel)

    def test_dropped_metadata_blocks_migration(self):
        self.assertFalse(self.inspect("")["sts_metadata_preserved"])

    def test_forwarded_metadata_passes_isolated_check(self):
        self.assertTrue(self.inspect(", meta=call_tool_request_params.meta")["sts_metadata_preserved"])

    def test_empty_metadata_does_not_pass(self):
        self.assertFalse(self.inspect(", meta={}")["sts_metadata_preserved"])


if __name__ == "__main__":
    unittest.main()

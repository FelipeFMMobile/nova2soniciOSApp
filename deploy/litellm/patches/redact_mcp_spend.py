"""Remove MCP payloads from LiteLLM spend metadata when prompt storage is off.

Pinned LiteLLM v1.103.0-dev.2 redacts messages/response but still copies the
MCP arguments and result into metadata.mcp_tool_call_metadata. Keep only the
tool/server/session attribution required by Usage/Spend.
"""

from pathlib import Path
import site


needle = '    clean_metadata["mcp_tool_call_metadata"] = mcp_tool_call_metadata\n'
replacement = '''    if mcp_tool_call_metadata is not None and not should_store_prompts_and_responses_in_spend_logs():
        mcp_tool_call_metadata = {
            key: value
            for key, value in mcp_tool_call_metadata.items()
            if key not in {"arguments", "result"}
        }
    clean_metadata["mcp_tool_call_metadata"] = mcp_tool_call_metadata
'''

candidates = [
    Path(root) / "litellm/proxy/spend_tracking/spend_tracking_utils.py"
    for root in site.getsitepackages()
]
target = next((path for path in candidates if path.is_file()), None)
if target is None:
    raise SystemExit("LiteLLM spend tracking module not found")
source = target.read_text()
if source.count(needle) != 1:
    raise SystemExit("unexpected LiteLLM spend tracking source; review pinned patch")
target.write_text(source.replace(needle, replacement))

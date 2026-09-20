"""Fix Nova 2 Sonic endpointing and tool arguments in pinned LiteLLM.

LiteLLM v1.103.0-dev.2 accepts OpenAI ``turn_detection`` in session.update but
does not translate it into Nova's sessionStart.turnDetectionConfiguration.
Without endpoint detection Nova can transcribe the microphone indefinitely
without opening an assistant audio turn. Keep the patch fail-closed so a future
image cannot silently retain an incompatible source layout.
"""

from pathlib import Path
import site


needle = '''                "sessionStart": {
                    "inferenceConfiguration": {
                        "maxTokens": self.max_tokens,
                        "topP": self.top_p,
                        "temperature": self.temperature,
                    }
                }
'''
replacement = '''                "sessionStart": {
                    "inferenceConfiguration": {
                        "maxTokens": self.max_tokens,
                        "topP": self.top_p,
                        "temperature": self.temperature,
                    },
                    "turnDetectionConfiguration": {
                        "endpointingSensitivity": "MEDIUM",
                    },
                }
'''

candidates = [
    Path(root) / "litellm/llms/bedrock/realtime/transformation.py"
    for root in site.getsitepackages()
]
target = next((path for path in candidates if path.is_file()), None)
if target is None:
    raise SystemExit("LiteLLM Bedrock Realtime transformation not found")
source = target.read_text()
if source.count(needle) != 2:
    raise SystemExit("unexpected LiteLLM Realtime source; review pinned patch")
patched = source.replace(needle, replacement)
if patched.count('"endpointingSensitivity": "MEDIUM"') != 2:
    raise SystemExit("LiteLLM Realtime turn detection patch was not applied")

tool_needle = '''        tool_input = {}
        if "input" in tool_use:
            try:
                tool_input = json.loads(tool_use["input"]) if isinstance(tool_use["input"], str) else tool_use["input"]
            except json.JSONDecodeError:
                tool_input = {}
'''
tool_replacement = '''        tool_input = {}
        # Nova 2 Sonic's bidirectional toolUse event carries the JSON payload in
        # ``content``. Retain ``input`` for compatibility with older variants.
        tool_input_value = tool_use.get("input", tool_use.get("content"))
        if tool_input_value is not None:
            try:
                tool_input = json.loads(tool_input_value) if isinstance(tool_input_value, str) else tool_input_value
            except json.JSONDecodeError:
                tool_input = {}
'''
if patched.count(tool_needle) != 1:
    raise SystemExit("unexpected LiteLLM toolUse source; review pinned patch")
patched = patched.replace(tool_needle, tool_replacement)
if patched.count('tool_use.get("input", tool_use.get("content"))') != 1:
    raise SystemExit("LiteLLM Realtime tool argument patch was not applied")

id_guard_needle = '''        if not current_output_item_id or not current_response_id:
            return [], "", ""

        # Parse the tool input
'''
id_guard_replacement = '''        # Nova can emit toolUse without an ASSISTANT contentStart. Do not drop the
        # call in that valid flow; synthesize OpenAI envelope identifiers while
        # preserving Bedrock's toolUseId as the function call identifier.
        if not current_output_item_id:
            current_output_item_id = f"item_{uuid.uuid4()}"
        if not current_response_id:
            current_response_id = f"resp_{uuid.uuid4()}"

        # Parse the tool input
'''
if patched.count(id_guard_needle) != 1:
    raise SystemExit("unexpected LiteLLM toolUse ID guard; review pinned patch")
patched = patched.replace(id_guard_needle, id_guard_replacement)
if patched.count("Nova can emit toolUse without an ASSISTANT contentStart") != 1:
    raise SystemExit("LiteLLM Realtime toolUse ID patch was not applied")
target.write_text(patched)

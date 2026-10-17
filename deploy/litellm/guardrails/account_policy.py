"""Deterministic account MCP policy. Never rewrite a signed envelope or log data."""
import json
import re
from litellm.integrations.custom_guardrail import CustomGuardrail
from litellm.types.guardrails import GuardrailEventHooks

TOOLS = {"identity.begin_login", "identity.login_status", "identity.logout", "profile.get_me", "profile.update_me"}
LEGACY_TOOLS = {"notes.create", "notes.list", "notes.delete", "agenda.list_slots", "agenda.list_events", "agenda.create_event", "agenda.cancel_event"}
FIELDS = {"name", "age", "city", "gender", "profession"}
DENIED = re.compile(r"(?i)(ignore (all |previous )?instructions|reveal (the )?(system prompt|secret)|ignore as instruções|revele (o prompt|os segredos))")

def reject():
    raise ValueError("account_policy_blocked")

def tool_name(data, tools=TOOLS):
    # Logging uses a different envelope on the post hook; only inspect metadata
    # keys, never user values, to identify the route.
    for key in ("mcp_tool_name", "name", "tool_name"):
        value = data.get(key)
        if isinstance(value, str):
            normalized = value.replace(".", "_").replace("-", "_")
            for tool in tools:
                suffix = tool.replace(".", "_")
                if normalized == suffix or normalized.endswith("_" + suffix):
                    return tool
    for key in ("metadata", "mcp_tool_call_metadata", "litellm_params", "standard_logging_object"):
        value = data.get(key)
        if isinstance(value, dict):
            found = tool_name(value, tools)
            if found:
                return found
    return None

def check_fields(changes):
    if not isinstance(changes, dict) or not changes or not changes.keys() <= FIELDS:
        reject()
    for key, value in changes.items():
        if value is None and key != "name":
            continue
        if key == "age":
            if type(value) is not int or not 0 <= value <= 130:
                reject()
        elif not isinstance(value, str) or len(value) > (80 if key == "gender" else 120) or key == "name" and not value.strip() or any(ord(c) < 32 for c in value) or DENIED.search(value):
            reject()

def check_input(name, envelope):
    if not isinstance(envelope, dict) or set(envelope) != {"payload", "host_context"}:
        reject()
    context, payload = envelope["host_context"], envelope["payload"]
    if not isinstance(context, dict) or set(context) != {"version", "audience", "tool", "payload_hash", "idempotency_key", "confirmed", "expires_at", "identity", "signature"} or context.get("version") != 2 or context.get("tool") != name:
        reject()
    identity = context.get("identity")
    if not isinstance(identity, dict) or set(identity) != {"subject", "client_code", "session_ref", "device_id", "scopes"}:
        reject()
    if not isinstance(payload, dict):
        reject()
    if name == "profile.update_me":
        if set(payload) != {"expected_version", "changes"} or type(payload["expected_version"]) is not int or payload["expected_version"] < 1:
            reject()
        check_fields(payload["changes"])
        if context.get("confirmed") is not True:
            reject()
    elif payload:
        reject()
    if name.startswith("profile."):
        identity = context.get("identity", {})
        if not re.fullmatch(r"[a-f0-9]{64}", identity.get("client_code", "")) or not identity.get("session_ref"):
            reject()

def check_output(name, text):
    try:
        value = json.loads(text)
    except (ValueError, TypeError):
        reject()
    if not isinstance(value, dict):
        reject()
    if value.get("status") == "error":
        if set(value) != {"status", "error"} or value["error"] not in {"authentication_required", "confirmation_required", "version_conflict", "idempotency_conflict", "profile_not_found", "invalid_host_context", "operation_rejected"}:
            reject()
        return
    if name.startswith("profile."):
        if set(value) != FIELDS | {"version"} or type(value["version"]) is not int or value["version"] < 1:
            reject()
        check_fields({key: value[key] for key in FIELDS})
    elif name == "identity.begin_login":
        if set(value) != {"status", "transaction_id"} or value["status"] != "login_required" or not re.fullmatch(r"[A-Za-z0-9_-]{43}", value["transaction_id"]):
            reject()
    elif set(value) != {"status"} or value["status"] not in {"anonymous", "authenticated", "signed_out"}:
        reject()

class AccountPolicy(CustomGuardrail):
    def get_supported_event_hooks(self):
        return [GuardrailEventHooks.pre_mcp_call, GuardrailEventHooks.post_mcp_call]

    async def apply_guardrail(self, inputs, request_data, input_type, logging_obj=None):
        name = tool_name(request_data)
        # Account routes take precedence even when an alias contains notes/agenda.
        # Legacy bypass accepts only the known catalog, including nested post metadata.
        if not name and tool_name(request_data, LEGACY_TOOLS):
            return inputs
        if not name:
            # Discovery text has no business payload. The account host requires
            # exactly the approved tool set and uses its own descriptions.
            if request_data.get("call_type") == "list_mcp_tools" or request_data.get("mcp_tool_description") is not None:
                for text in inputs.get("texts", []):
                    if DENIED.search(text):
                        reject()
                return inputs
            # This guardrail belongs to the dedicated account LiteLLM stack;
            # missing routing context must not silently disable it.
            reject()
        if input_type == "request":
            check_input(name, request_data.get("mcp_arguments", request_data.get("arguments")))
        else:
            texts = inputs.get("texts", [])
            if not texts:
                reject()
            for text in texts:
                check_output(name, text)
        return inputs

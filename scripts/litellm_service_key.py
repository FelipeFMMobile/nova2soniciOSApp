#!/usr/bin/env python3
"""Create a least-privilege local LiteLLM key and print its gateway exports."""
import argparse
import json
import shlex
import urllib.request

SERVERS = ["sts-notes", "sts-agenda"]
TOOLS = {
    "sts-notes": ["notes.create", "notes.list", "notes.delete"],
    "sts-agenda": ["agenda.list_slots", "agenda.list_events", "agenda.create_event", "agenda.cancel_event"],
}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:4000")
    parser.add_argument("--master-key", required=True)
    args = parser.parse_args()
    body = json.dumps({
        "key_alias": "sts-go-gateway",
        "models": ["nova-sonic"],
        "allowed_routes": ["/v1/realtime", "mcp_routes"],
        "object_permission": {"mcp_servers": SERVERS, "mcp_tool_permissions": TOOLS},
        "mcp_rpm_limit": {"memo": 60, "local": 60},
    }).encode()
    request = urllib.request.Request(args.url.rstrip("/") + "/key/generate", data=body, method="POST", headers={"Authorization":"Bearer " + args.master_key, "Content-Type":"application/json"})
    with urllib.request.urlopen(request, timeout=15) as response:
        value = json.load(response)
    key = value.get("key")
    if not key: parser.exit(1, "LiteLLM did not return a service key\n")
    print("export STS_MCP_BACKEND=litellm")
    print("export STS_LITELLM_URL=" + shlex.quote(args.url.rstrip("/")))
    print("export STS_LITELLM_REALTIME_MODEL=nova-sonic")
    print("export STS_LITELLM_API_KEY=" + shlex.quote(key))
    servers = json.dumps([
        {"alias":"memo", "server_id":"sts-notes", "allowed_tools":TOOLS["sts-notes"], "policies":{"notes.create":"explicit_intent", "notes.list":"read_only", "notes.delete":"confirm_later"}},
        {"alias":"local", "server_id":"sts-agenda", "allowed_tools":TOOLS["sts-agenda"], "policies":{"agenda.list_slots":"read_only", "agenda.list_events":"read_only", "agenda.create_event":"explicit_intent", "agenda.cancel_event":"confirm_later"}},
    ], separators=(",", ":"))
    print("export STS_LITELLM_MCP_SERVERS=" + shlex.quote(servers))

if __name__ == "__main__": main()

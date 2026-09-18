#!/usr/bin/env python3
"""Offline compatibility gate against the actual LiteLLM wheel's client code.

Executes the SDK call operation with a capturing session, without importing or
starting the proxy. This is not a full proxy/network acceptance test.
"""
import argparse
import ast
import asyncio
import hashlib
import json
from types import SimpleNamespace
import zipfile


CLIENT = "litellm/experimental_mcp_client/client.py"
REQUIRED = {"sts/idempotencyKey": "synthetic-operation", "sts/confirmed": True}


def inspect_wheel(path):
    with zipfile.ZipFile(path) as archive:
        tree = ast.parse(archive.read(CLIENT).decode())
        metadata = archive.read(next(n for n in archive.namelist() if n.endswith(".dist-info/METADATA"))).decode()
    version = next(line.removeprefix("Version: ") for line in metadata.splitlines() if line.startswith("Version: "))
    classes = [n for n in tree.body if isinstance(n, ast.ClassDef)]
    operations = []
    for cls in classes:
        for method in cls.body:
            if isinstance(method, ast.AsyncFunctionDef) and method.name == "call_tool":
                operations.extend(n for n in method.body if isinstance(n, ast.AsyncFunctionDef) and n.name == "_call_tool_operation")
    if len(operations) != 1:
        raise ValueError("client implementation changed; manual compatibility review required")
    operation = operations[0]
    operation.decorator_list = []
    operation.returns = None
    for arg in operation.args.args:
        arg.annotation = None
    module = ast.fix_missing_locations(ast.Module(body=[operation], type_ignores=[]))
    captured = {}

    class Session:
        async def call_tool(self, **kwargs):
            captured.update(kwargs)

    # Supply metadata on the request to detect whether the client forwards it,
    # even though the gateway's request constructor currently omits it as well.
    params = SimpleNamespace(name="notes.delete", arguments={"id": "synthetic-note"}, meta=REQUIRED)
    namespace = {
        "call_tool_request_params": params,
        "on_progress": None,
        "verbose_logger": SimpleNamespace(debug=lambda *args: None),
    }
    exec(compile(module, CLIENT, "exec"), namespace)
    asyncio.run(namespace["_call_tool_operation"](Session()))
    received = captured.get("meta")
    if hasattr(received, "model_dump"):
        received = received.model_dump(by_alias=True)
    passed = isinstance(received, dict) and all(received.get(k) == v for k, v in REQUIRED.items())
    with open(path, "rb") as wheel:
        digest = hashlib.file_digest(wheel, "sha256").hexdigest()
    return {"version": version, "wheel_sha256": digest, "sdk_call_fields": sorted(captured), "sts_metadata_preserved": passed, "method": "isolated execution of wheel client SDK operation; no live proxy"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("wheel", help="official LiteLLM .whl archive")
    args = parser.parse_args()
    try:
        report = inspect_wheel(args.wheel)
    except (ValueError, KeyError, SyntaxError, StopIteration, OSError, zipfile.BadZipFile) as error:
        parser.exit(2, f"Compatibility inspection failed: {error}\n")
    print(json.dumps(report, indent=2))
    return 0 if report["sts_metadata_preserved"] else 1


if __name__ == "__main__":
    raise SystemExit(main())

# STS Model POC

Proof of concept for a Brazilian Portuguese speech-to-speech assistant. The
Apple client streams microphone audio to a local Go gateway. The gateway owns
session state, interruption, permissions, and MCP tool execution, while
Qwen3-Omni inference runs on a remote RunPod GPU.

## Prerequisites

- macOS 26 with Xcode 27
- Go 1.27 or newer
- Docker for local container validation
- A RunPod account for the Qwen integration stages

## Local commands

```bash
cp .env.example .env
make check
make gateway
```

The default `fake` provider requires no external service. Runtime data and
secrets are intentionally excluded from Git.

## Repository layout

- `apps/apple`: shared SwiftUI application for iOS and macOS.
- `cmd/gateway`: public HTTP/WebSocket gateway.
- `cmd/mcp-notes`: local MCP notes server.
- `internal`: protocol, provider, orchestration, audio, and persistence code.
- `deploy/runpod`: remote inference container and operating runbook.
- `tests/e2e`: black-box scenarios and performance harnesses.

See [Architecture](docs/architecture.md) for responsibilities and trust
boundaries.


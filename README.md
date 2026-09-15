# STS Model POC

Proof of concept for a Brazilian Portuguese speech-to-speech assistant. The
Apple client streams microphone audio to a local Go gateway. The gateway owns
session state, interruption, permissions, and MCP tool execution, while Amazon
Nova 2 Sonic provides managed bidirectional inference through AWS Bedrock.

## Prerequisites

- macOS 26 with Xcode 27
- Go 1.27 or newer
- Python 3.12 for the small Bedrock transport bridge
- An AWS account with Nova 2 Sonic access in `us-east-1`

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
- `services/nova-bridge`: local adapter for the AWS bidirectional SDK.
- `deploy/aws`: Bedrock IAM policy and setup runbook.
- `tests/e2e`: black-box scenarios and performance harnesses.

See [Architecture](docs/architecture.md) for responsibilities and trust
boundaries.

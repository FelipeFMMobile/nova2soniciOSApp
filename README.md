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
make check
export STS_PROVIDER=fake
export STS_DEVELOPMENT_TOKEN=local-demo-token
make gateway
```

In another terminal, export the same token, then run:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make voice-demo
make voice-cancel
```

The normal demo saves `/private/tmp/sts-fake-response.wav`. It is a 440 Hz
tone, not speech: the fake provider does not perform ASR or call AWS. The
cancellation demo discards its output. Use `go run ./cmd/voice-client -help`
for an alternate URL, input WAV, or output path.

Configuration comes from process environment variables. `.env.example` is a
reference template; `.env` is **not automatically loaded**. Development may run
without a token only on loopback. Non-loopback bindings and production require
a token. Runtime data and secrets are intentionally excluded from Git.

See [Voice protocol](docs/voice-protocol.md) for endpoints, event examples,
limits, latency definitions, and the Stage 2 validation results.

## Repository layout

- `apps/apple`: shared SwiftUI application for iOS and macOS.
- `cmd/gateway`: public HTTP/WebSocket gateway.
- `cmd/voice-client`: terminal demonstration and WAV capture.
- `cmd/mcp-notes`: local MCP notes server.
- `internal`: protocol, provider, orchestration, audio, and persistence code.
- `services/nova-bridge`: local adapter for the AWS bidirectional SDK.
- `deploy/aws`: Bedrock IAM policy and setup runbook.
- `tests/e2e`: black-box scenarios and performance harnesses.

See [Architecture](docs/architecture.md) for responsibilities and trust
boundaries.

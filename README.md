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

## Real Nova 2 Sonic through the gateway (Stage 3)

Use three terminals. The local token is your choice and is unrelated to AWS
credentials. The Python bridge uses the AWS profile already authorized for
Bedrock; set `AWS_PROFILE` only if you need a named profile.

Terminal 1 — private Python/Bedrock bridge:

```bash
make nova-install
make nova-bridge
```

Terminal 2 — public Go gateway:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
export STS_PROVIDER=nova
make gateway
```

Terminal 3 — send an actual PT-BR WAV (mono PCM16, 16 kHz):

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make nova-demo WAV=/absolute/path/input-pt-br.wav
make nova-barge-demo WAV=/absolute/path/input-pt-br.wav BARGE_WAV=/absolute/path/interruption-pt-br.wav
```

The first command saves the spoken response. The second injects the second
recording during the first response, requires native Nova interruption, clears
old output, and saves only the new response. These commands incur Bedrock
usage charges. Stop the Go gateway and Python bridge with Ctrl+C when finished.
Recordings are ignored by Git. See [Nova integration](docs/nova-integration.md)
for session behavior, limitations, and the live validation report.

## Nova with MCP tools (before the Apple apps)

Keep the Nova bridge running, then replace `make gateway` with:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make mcp-gateway
```

Go discovers the local notes server's tools and presents them to Nova. The
model requests tools; Go executes MCP calls and returns their actual results
for spoken responses. Notes create/list/delete, durable retry, and subsequent
voice confirmation before deletion have passed real AWS validation.

The terminal supports `-expect-tool notes.list`, `-request-id` for safe retry,
`-events /private/tmp/events.jsonl` for private evidence, and `-followup-wav`
for a second voice turn. No SwiftUI app is needed for this demonstration.
See [MCP setup, safety and live results](docs/mcp-integration.md).

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

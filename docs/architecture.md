# Architecture

## Runtime topology

```text
iOS/macOS app
    | local WebSocket (versioned JSON events + PCM16/base64)
    v
Go gateway on the Mac
    |-- session state, permissions, and MCP orchestration
    |-- MCP client --> local notes MCP server --> SQLite
    `-- local WebSocket --> Python transport bridge
                              `-- SigV4 HTTP/2 --> Bedrock --> Nova 2 Sonic
```

The Apple app never receives AWS credentials and never connects to Bedrock
directly. The gateway is the only security and orchestration boundary exposed
to the client. A minimal Python process owns only the AWS bidirectional stream
because that API is not currently available in the AWS SDK for Go.

## Component responsibilities

### Apple app

Captures voice, performs voice processing and sample conversion, renders
session state and transcripts, and plays ordered response audio. It stops
playback immediately when the gateway reports an interruption.

### Gateway

Authenticates the development client, validates protocol events, owns the
conversation state machine, cancels stale model work, invokes permitted MCP
tools, and emits structured logs and latency measurements.

### Nova provider and transport bridge

The Go provider adapts the stable internal session interface to a private local
WebSocket. The Python bridge signs and exchanges Bedrock event-stream messages
without owning business rules. Nova performs speech understanding, response
generation, speech synthesis, turn-taking, barge-in, and asynchronous tool
requests in one bidirectional session. A deterministic fake provider remains
available for local tests.

### MCP notes server

Exposes create, list, and delete operations. The orchestrator validates every
call, makes creation idempotent, and requires confirmation before deletion.

## Audio contract

- Input: signed 16-bit little-endian PCM, mono, 16 kHz.
- Transport: base64 audio chunks inside versioned JSON WebSocket events.
- Output: sample rate declared by the provider event; clients must not assume
  a fixed value.
- Cancellation: every chunk belongs to a turn and sequence. Chunks belonging
  to a cancelled or older turn are discarded.

## Security defaults

- Development services bind to loopback unless explicitly configured.
- AWS credentials come from the standard profile/SSO credential chain.
- Long-lived AWS access keys are not stored in `.env`.
- Audio payloads, transcripts, tokens, and note contents are not logged.
- Runtime data, databases, recordings, and model weights are ignored by Git.

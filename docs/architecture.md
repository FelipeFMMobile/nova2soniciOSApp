# Architecture

## Runtime topology

```text
iOS/macOS app
    | local WebSocket (versioned JSON events + PCM16/base64)
    v
Go gateway on the Mac
    |-- session state and barge-in
    |-- MCP client --> local notes MCP server --> SQLite
    `-- authenticated HTTPS/WSS --> RunPod --> vLLM-Omni --> Qwen3-Omni
```

The Apple app never receives RunPod credentials and never connects to the
inference service directly. The gateway is the only security and orchestration
boundary exposed to the client.

## Component responsibilities

### Apple app

Captures voice, performs voice processing and sample conversion, renders
session state and transcripts, and plays ordered response audio. It stops
playback immediately when the gateway reports an interruption.

### Gateway

Authenticates the development client, validates protocol events, owns the
conversation state machine, cancels stale model work, invokes permitted MCP
tools, and emits structured logs and latency measurements.

### Voice provider

Adapts the stable internal session interface to Qwen's vLLM-Omni endpoints.
The provider is replaceable and has a deterministic fake implementation for
local tests.

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
- RunPod and local development tokens are read only from the environment.
- Audio payloads, transcripts, tokens, and note contents are not logged.
- Runtime data, databases, recordings, and model weights are ignored by Git.


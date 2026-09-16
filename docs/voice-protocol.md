# Voice gateway — protocol v1

Stage 2 introduced local transport and a deterministic fake provider. Stage 3
adds the Nova adapter; see [Nova integration](nova-integration.md). The fake
provider still does not invoke Nova, execute MCP tools, capture a microphone,
or synthesize speech.

## Endpoints

| Endpoint | Authentication | Purpose |
| --- | --- | --- |
| `GET /healthz` | Public | Process liveness and protocol version |
| `GET /v1/providers` | Bearer when configured | Available adapters and audio rates |
| `WS /v1/voice` | Bearer when configured | One fresh session per connection |
| `GET /v1/metrics` | Bearer when configured | Counters and fake first-audio timing |

`STS_DEVELOPMENT_TOKEN` is sent in the `Authorization: Bearer ...` header,
never in a query string. Browser WebSockets must have the same host in their
Origin header; native clients may omit Origin. There is no broad CORS bypass.

The default provider is `fake`. Set `STS_PROVIDER=nova` for the real Stage 3
adapter. Adapter availability in `/v1/providers` is not an AWS health/access
probe; `session.ready` is emitted only after the private bridge handshake.
Provider failure never silently falls back to fake output. The Python bridge
from Stage 1 remains independently usable.

## Lifecycle

1. Open the socket; send `session.start` with version 1 and optional provider.
2. Receive `session.ready`; copy the server-generated `sessionId`.
3. Choose a new `turnId`, send input audio with contiguous sequence numbers.
4. Send `turn.commit` to generate a fake response.
5. Receive transcripts, ordered output audio, `turn.completed` and idle state.
6. Repeat with a new turn ID, or send `session.stop` and receive
   `session.stopped` followed by a normal WebSocket close (1000).

Example input (the short audio payload represents two zero-valued samples):

```json
{"version":1,"type":"session.start","provider":"fake"}
{"version":1,"type":"audio.append","sessionId":"SERVER_ID","turnId":"turn-1","sequence":1,"sampleRate":16000,"audio":"AAAAAA=="}
{"version":1,"type":"turn.commit","sessionId":"SERVER_ID","turnId":"turn-1"}
```

Output events include `session.ready`, `session.state`, `turn.started`,
`transcript` (USER or ASSISTANT), `audio.output`, `turn.completed`,
`turn.interrupted`, `session.stopped`, and `error`. All carry version and session
ID; turn events carry turn ID. Audio output sequence starts at 1 for each turn.
Transcripts have no audio sequence. The fake transcript is explicitly labeled
as simulated; fake output is 0.8 seconds of tone at 24 kHz.

`tool.started`, `tool.result`, and `tool.confirmation` are reserved server
events in the Go contract for Stage 6; tools are not executable in Stage 2.

## Cancellation and ordering

Send `turn.cancel` with the active turn ID to cancel listening or generation.
The gateway sends `turn.interrupted` and returns to idle. Alternatively,
`audio.append` with an unused turn ID and sequence 1 interrupts a responding
turn and starts listening to the new one. A malformed new frame does not
cancel valid work.

The gateway cancels the old provider context and drops its queued events.
Clients must also clear playback and reject old turn IDs or audio sequences
after `turn.interrupted`. Reusing old turn IDs, duplicate/gapped input sequences,
or mixing session IDs generates a recoverable error without changing the
valid conversation state. IDs use 1–128 ASCII letters, digits, `_` or `-`.

The fake adapter is turn-based and uses manual `turn.commit`. Nova forwards
each chunk immediately and uses model VAD. For Nova, `turn.commit` is a timing
marker, not an inference trigger. A continuous microphone container can keep
its input ID/sequence while Nova generates fresh output turn IDs after native
interruption. Track server `turn.started` events for output playback. Nova
transcripts declare `stage` (SPECULATIVE or FINAL); only FINAL spoken text is
used in renewal history. `session.renewed` signals private-session replacement.

## Limits and failure behavior

| Limit | Default |
| --- | --- |
| Concurrent sessions (`STS_MAX_SESSIONS`) | 16; overflow returns HTTP 503 |
| Event bytes (`STS_MAX_EVENT_BYTES`) | 524,288; overflow closes with 1009 |
| PCM chunk | 32,000 decoded bytes; PCM16 little-endian mono, 16 kHz |
| Buffered input per turn | 30 seconds / 960,000 bytes |
| Turns per connection | 128; reconnect creates a fresh session |
| Session start (`STS_READ_TIMEOUT`) | 15 seconds |
| Write (`STS_WRITE_TIMEOUT`) | 15 seconds |
| Socket inactivity (`STS_IDLE_TIMEOUT`) | 90 seconds; ping/pong heartbeat |
| Provider turn (`STS_TURN_TIMEOUT`) | 30 seconds |

32 ms / 1,024-byte input chunks are recommended for later live audio. Base64
must decode to a nonempty, even-sized PCM16 buffer. Unknown JSON fields,
unsupported versions, binary frames, and client attempts to set server fields
are rejected. Invalid numeric limits and non-positive timeouts fail startup.

Provider timeout emits `provider_timeout` and releases the turn. Disconnect,
session stop, or SIGINT/SIGTERM cancels all work and releases the socket and
buffer. Reconnection intentionally starts a new context; persistence and
automatic recovery are Stage 7 work. HTTP shutdown also explicitly closes
hijacked WebSockets and waits for their reader/provider goroutines.

## Metrics and local validation

`turn.completed.metrics.firstAudioMs` measures from gateway turn commit to
the first provider audio frame, not microphone end-of-speech or useful speech.
`durationMs` measures provider generation completion. `interruptionMs` measures
gateway-local cancellation processing before the notification; it excludes
network transport, Apple playback, and microphone response time.

Authenticated `/v1/metrics` exposes session/turn/error counts, active sessions,
first-audio sample count and mean. Logs contain identifiers and timing only;
audio, transcripts, notes, and tokens are not logged.

On 2026-09-16 the terminal demo received 20 chunks / 38,400 bytes and saved an
0.8-second WAV. First fake audio arrived at 41.103 ms; completion was 825.127
ms. A second run cancelled at 120 ms and discarded its output. Afterwards:
zero active sessions, one completed turn, one interrupted turn, zero errors.
SIGINT shutdown exited normally. These are fake-provider transport checks,
not Nova latency benchmarks or Apple-device validation.

Automated coverage includes HTTP auth/origin checks, complete ordered audio,
cancel/barge-in stale rejection, malformed frames, session mismatch, capacity,
oversized frames, deadlines, reconnect, 12 concurrent disconnects, shutdown
with active workers, and terminal WAV/cancellation scenarios. Run `make check`
for build, tests, race detector, and vet. Tests require permission to bind
temporary local ports; no AWS service is used.

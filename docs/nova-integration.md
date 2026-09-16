# Stage 3 — Nova 2 Sonic through the Go gateway

## What was already done and what changed

Stage 1 validated a WAV client directly against the Python transport bridge and
AWS. Stage 2 validated the Go gateway with fake output. Neither was a real
gateway-to-Nova integration. Stage 3 joins the paths:

```text
terminal WAV / future Apple microphone
    -> Go public WebSocket + authentication/session control
    -> private Python bridge
    -> SigV4 Bedrock Nova 2 Sonic
    -> Go transcript/audio mapping
    -> client PCM playback / WAV
```

The operational plan uses Nova exclusively; any Qwen/RunPod mention in Git
history records the abandoned approach, not a live deployment dependency.

## Streaming and turns

`STS_PROVIDER=nova` selects a real adapter, never fake fallback. Each client
connection gets its own private Bedrock stream. The gateway forwards validated
16 kHz mono PCM16/base64 input immediately, including while a response is
streaming. It does not accumulate a WAV or wait for `turn.commit` before
forwarding input. `turn.commit` is an optional timing marker; Nova VAD determines
when to respond. Output is PCM16 mono at 24 kHz, declared on every audio event.

The mapper associates Nova `contentId` values with internal output turns. It
maps USER/ASSISTANT transcripts with FINAL/SPECULATIVE stages, sequences audio,
and ends responses on END_TURN audio/final-text/completion boundaries. A
PARTIAL_TURN audio boundary does not prematurely end a response.

For continuous input, reuse the microphone container's input ID and increasing
sequence. Nova detects new speech, sends an interruption signal, and continues
the conversation in the same stream. The gateway emits `turn.interrupted`,
blocks old content IDs, and emits a fresh `turn.started` output ID. The client
must clear old playback; input container IDs and new output IDs can differ.
Explicitly changing the input ID also clears the prior response immediately,
then continues audio so Nova can perform its server-side turn transition.

`turn.cancel` cancels the active internal turn immediately and replaces the
private stream using finalized history. After `session.renewed`, send a new
unused input ID starting at sequence 1. This explicit hard cancellation differs
from native speech barge-in, which does not reconnect. No fictional
`response.cancel` message is sent to Bedrock.

## Renewal, bounds, errors

Private streams normally renew at `STS_NOVA_MAX_SESSION_AGE=7m30s`, before
Bedrock's eight-minute session limit. If a turn is active, renewal waits in
10-second increments, but at 7m50s interrupts and renews rather than running
into the hard limit. The public client session ID remains unchanged. Input
can pause during the bridge handshake; bounded socket queues/backpressure
prevent unbounded buffering. `session.renewed` informs the client of the change.

History includes only finalized USER text and actually spoken FINAL ASSISTANT
text, never speculative or cancelled generations. Multiple final ASR fragments
are concatenated. At most 32 messages / 64 KiB are retained and sent between
the system prompt and audio start. This preserves textual context, not the
original acoustic/prosodic state. Connections retain a maximum of 128 turns;
clients must reconnect after the configured session turn bound.

The Nova path does not buffer microphone audio for 30 seconds like fake; chunk
bounds remain in force. Provider generation timeout and disconnect generate
controlled errors and release sockets. Startup cancellation closes the private
connection if the public client disconnects. Raw inference errors and AWS
credentials do not reach public clients; audio/transcripts/tokens are not
logged by Go. Native tool events are mapped, but tools are not advertised or
executed until the MCP stage.

The AWS Python SDK is experimental. AWS CRT 0.28.4 emitted cancelled-Future
warnings on premature stream teardown during the live tests; bridge close now
drains the in-flight read for up to two seconds before cancellation. That
adjustment is unit-tested; its updated teardown has not been re-tested live.
Long-duration outage/renewal soak testing remains a documented follow-up for
the resilience/release stages; no dependency files in `.venv` were patched.

## Validation on 2026-09-16

Two live sessions ran against `amazon.nova-2-sonic-v1:0` in `us-east-1`, using
the PT-BR `carolina` voice and the account permissions authorized by the user.

| Scenario | Result | Response WAV | First audio |
| --- | --- | --- | --- |
| WAV -> Go -> Nova -> Go -> WAV | Passed; intelligible PT-BR response received | 170,880 bytes / 3.56 s | 1,216.741 ms after commit |
| Second speech in the same input stream | Passed; native interruption, stale output discarded, new spoken response | 122,880 bytes / 2.56 s | 813.794 ms from new ASR turn start |

The interruption request was “Espere. Agora responda somente bom dia.” The
model responded “Bom dia! Como posso ajudar no que precisar?”. It did not
strictly obey the request for only two words, which is a model behavior
limitation, not a tool success claim.

Final local metrics: two public sessions, two completed responses, one native
interruption, zero active sessions, zero public errors, three first-audio
samples (including the interrupted response). Both processes were stopped;
no merge/tag for Stage 3 was performed.

These are individual observations with different timing origins, not a latency
median, a 95% benchmark, or proof of the 300 ms Apple playback requirement.
Live renewal at eight minutes and hardware microphone/echo cancellation have
not been tested yet.

`make check` covers build, tests, race detector and vet with simulated bridges:
audio before commit, native interrupt, hard cancel, history renewal, provider
loss, bounded read queues, stale PCM, FINAL-only history, and terminal barge-in
WAV capture. `make nova-test` verifies the Python session/audio/history contract.
Use `make nova-demo WAV=...` or `make nova-barge-demo WAV=... BARGE_WAV=...` for
paid live reproduction; see the README for the three-terminal setup.

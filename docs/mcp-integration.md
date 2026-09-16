# Nova 2 Sonic + local MCP — Stage 6

Stage 6 runs before the Apple apps. It is implemented on `stage/06-mcp-notes`,
accepted by the user for merge/tag `v0.5.0`. Stage 3 was accepted and merged
as `v0.4.0`. GitHub publication was authorized; the destination is configured
separately from the local implementation.

## What the model can do

Nova chooses tools with automatic tool choice. Go discovers allowed MCP tools,
converts their schemas into native Nova configuration, validates each call,
executes the local MCP server, and returns the result to the same bidirectional
conversation. Nova does not establish an MCP connection itself.

```text
PT-BR WAV client → Go gateway → Python transport → Nova / Bedrock
                      ↑                           │ toolUse
                      └───────────────────────────┘
                      │ tools/call (MCP stdio)
                      ↓
                 mcp-notes → SQLite
                      │ result
                      └→ Go → Python → Nova → spoken response → client
```

MCP names `notes.create`, `notes.list`, `notes.delete` become `notes_create`,
`notes_list`, `notes_delete` for Nova. Names/arguments are allowlisted and
validated against the discovered JSON schemas. Conflicting mapped names fail
discovery. Schemas with remote references are rejected, not downloaded.

The MCP subset is pinned to **2025-11-25**, with initialize, initialized
notification, tools/list and tools/call over newline-delimited JSON-RPC stdio.
Newer stateless MCP, HTTP servers, resources, prompts, sampling, pagination and
dynamic tool-list updates are not implemented. Only bounded text tool results
are returned to Nova (64 KiB maximum).

## Start the demonstration

Run from the repository root. `.env.example` is a template, not automatically
loaded. Export variables explicitly. Use your existing AWS profile; `sts-poc`
in older examples is only a suggested profile name. The live validation used
the machine's configured `default` profile without changing IAM or credentials.

Terminal 1:

```bash
export AWS_PROFILE=default
make nova-bridge
```

Terminal 2:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
make mcp-gateway
```

This builds `bin/mcp-notes` and starts Go with that executable configured. Go
starts and initializes a separate MCP subprocess for each public voice session;
subprocesses share the local `data/sts.sqlite` database and close with sessions.
Do not also start `make mcp` expecting a TCP service: it is a stdio process,
not a network listener. No additional AWS policy is required for local notes.

Terminal 3, using mono PCM16 16 kHz WAV input:

```bash
export STS_DEVELOPMENT_TOKEN=local-demo-token
go run ./cmd/voice-client -provider nova \
  -wav /absolute/path/create-note.wav \
  -request-id demo-create-1 -expect-tool notes.create \
  -events /private/tmp/demo-create-events.jsonl \
  -output /private/tmp/demo-create-response.wav
```

Suggested speech: “Crie uma nota com o título Código Aurora e conteúdo: o
código é sete quatro um nove.” In a **new connection**, ask: “Busque nas minhas
notas e me diga o código da nota Código Aurora”, with a new request ID and
`-expect-tool notes.list`. That request does not contain the answer, proving
retrieval rather than reuse of the original conversation's context.

For deletion, use “Exclua a nota que tem o título Código Aurora” and provide
`-followup-wav /absolute/path/confirm.wav`, containing exactly “Confirmo excluir”.
Add `-expect-tool notes.list,notes.delete`. The client sends the follow-up after
the first response and saves that first response as `OUTPUT.turn-1.wav`.
Using “Não confirmo” instead must leave the note intact; do not expect a
successful delete in that scenario. `confirmation_required` never satisfies
the client's expected successful tool check.

Backend logs contain tool IDs/names/status, not audio, transcripts or arguments.
The terminal intentionally displays transcripts and tool contents. Optional
JSONL captures include sensitive data and audio; keep them private, outside Git.
WAV, CAF, PCM, JSONL and SQLite journal files are ignored. Stop both services
with Ctrl+C after the demo; calls incur Bedrock usage charges.

## Plug another server

Configure one trusted stdio executable and an explicit allowlist:

```bash
export STS_MCP_COMMAND=/absolute/path/to/trusted-mcp-server
export STS_MCP_ARGS='["argument","value"]'
export STS_MCP_ALLOWED_TOOLS=lookup_record
export STS_MCP_TIMEOUT=10s
export STS_PROVIDER=nova
make gateway
```

The command is never supplied by the model and is executed directly, without
a shell. A server must support the pinned handshake and text tools subset.
Unknown non-notes tools require confirmation with “confirmo executar” by
default, regardless of the server's untrusted `readOnlyHint` annotation. This
is conservative. The additional [Notes + Agenda stage](multi-mcp-agenda.md)
implements explicit host policies and multiple servers while preserving this
legacy configuration.
Configured subprocesses are trusted local code, **not sandboxed**: they inherit
the host process environment and OS user permissions. Review before configuring.
Generic servers may ignore the host retry metadata; the no-duplicate guarantee
below is specific to the notes server, not universal MCP behavior.

## Confirmation, retry and lifecycle

- Deletion first returns `confirmation_required`, emits `tool.confirmation`,
  and does **not** call the delete tool on MCP. Approval is bound to the exact
  target/arguments and expires after 60 seconds.
- Go observes only finalized USER ASR, concatenates fragments, and evaluates
  the complete utterance before the next TOOL/ASSISTANT block. USER content
  can end with `PARTIAL_TURN`; it is not the end of the assistant response.
- The exact phrase in a subsequent turn authorizes the **next matching**
  deletion call. A bare “sim”, a quoted phrase, assistant text or model-supplied
  `confirmed` argument cannot authorize it. Refusal clears pending approval.
  This is a POC safeguard, not voice identity verification or production auth.
- Trusted clients may alternatively send `tool.confirm` with operationId and
  boolean approved; they cannot choose a different tool/target in that event.
- `requestId` is a client-generated logical request identity in session.start.
  Reuse it on retry; choose a fresh ID for a deliberately new mutation. The
  terminal generates one unless `-request-id` is supplied. This POC permits
  one notes creation and one deletion per logical request. Reusing a request ID
  with changed arguments fails instead of creating an extra note.
- The notes server receives idempotency/approval through host-only `_meta`,
  never through the model schema. SQLite commits each mutation and its retry
  result atomically. Replay survives subprocess/gateway restarts.
- Notes and the durable effect ledger are in `notes` and `operations`. Go
  persists metadata in `sessions`, `turns`, `tool_operations`; unfinished calls
  become `outcome_unknown` on disconnection, not invented success. Set
  `STS_DATABASE_PATH` and the notes server `-db` to the same path if desired;
  custom server data may otherwise be separate from the gateway audit database.
- At most four tool jobs are queued per voice session; microphone frames keep
  flowing while tools execute. Nova results are returned only after its TOOL
  content ends. Failures produce native error tool results so Nova is not left
  waiting. Old/cancelled staged calls do not execute.
- A stdio timeout closes that MCP transport to discard late responses; start a
  new public session to reconnect. Tools are re-advertised on Nova renewal, but
  renewal is deferred while jobs run. Explicit cancel during an in-flight call
  closes the session with a controlled pending-outcome error; retry the same
  logical request. Longer failure recovery belongs to Stage 7.

## Live validation — 2026-09-16

Real AWS Nova, real Go gateway, real stdio MCP subprocess and real SQLite:

| Scenario | Evidence |
| --- | --- |
| Voice creation | `notes_create` → MCP `notes.create` → status `created`; one note |
| Retrieval in a fresh conversation | `notes_list` returned stored note; Nova spoke code 7419 |
| Creation retry in another connection | Same note ID and creation timestamp; one durable creation |
| Deletion refusal | Pending confirmation; USER refused; note remained |
| Confirmed deletion | Pending → new FINAL USER “confirmo excluir” → MCP delete → spoken success |
| Query after deletion, final audited build | Empty notes result; Nova did not invent the missing code; session/turn/tool closure persisted |

Final test database `/private/tmp/sts-mcp-live.sqlite` contains zero notes,
one `notes.create` effect and one `notes.delete` effect. Only the synthetic test
note was removed, recoverable from the local create event capture; no user note
was touched. Original captures/audio are local test artifacts, not Git assets.

| Saved response | PCM16 mono 24 kHz bytes | Audio duration | First audio metric |
| --- | ---: | ---: | ---: |
| `/private/tmp/sts-mcp-create-response.wav` | 353280 | 7.36 s | 2195 ms |
| `/private/tmp/sts-mcp-query-response.wav` | 716160 | 14.92 s | 2354 ms |
| `/private/tmp/sts-mcp-retry-response.wav` | 343680 | 7.16 s | 2053 ms |
| `/private/tmp/sts-mcp-refusal-response.wav` | 624000 | 13.00 s | 809 ms |
| `/private/tmp/sts-mcp-delete-response.wav` | 174720 | 3.64 s | 1267 ms |
| `/private/tmp/sts-mcp-empty-response.wav` | 556800 | 11.60 s | 2089 ms |

These are individual observations, not a benchmark median. Initial metrics are
commit-based; follow-up metrics are native ASR-turn-based. Tool-backed initial
responses exceeded the 1.5-second release target. Generation duration differs
from audio playback duration. Nova was more verbose than requested and ASR
transcribed “o código” as “ou código” in the creation; benchmark/PT-BR accuracy
and microphone echo/barge-in validation remain future work.

Two compatibility issues were found and fixed during live testing: Bedrock
requires `inputSchema.json` as a serialized JSON string, and USER ASR ended
with `PARTIAL_TURN`. The initial rejected schema session did not send audio;
the initial confirmation attempt was safely blocked and did not delete data.
The corrected confirmation succeeded. A first profile lookup failed locally
because the example profile did not exist; the configured default profile worked.

Repeat artifact verification without AWS calls:

```bash
python3 tests/e2e/mcp_voice_evidence.py
make check
make nova-test
```

The evidence script expects this report's six capture files and isolated
database; it is not a general benchmark runner. Automated tests additionally
exercise real stdio + SQLite across two gateway sessions, schema rejection,
host confirmation, expiry/refusal, unknown tools, timeout, native audio behavior
and durable replay. Full long-session renewal with tool history, live network
failure/reconnect, ASR diversity and hardware testing are not yet claimed.

References: [AWS tools](https://docs.aws.amazon.com/nova/latest/nova2-userguide/sonic-tool-configuration.html),
[AWS output events](https://docs.aws.amazon.com/nova/latest/nova2-userguide/sonic-output-events.html),
[AWS wire-format sample](https://github.com/aws-samples/amazon-nova-samples/blob/main/speech-to-speech/amazon-nova-2-sonic/sample-codes/console-python/nova_sonic_with_text.py),
[MCP pinned schema](https://modelcontextprotocol.io/specification/2025-11-25/schema).

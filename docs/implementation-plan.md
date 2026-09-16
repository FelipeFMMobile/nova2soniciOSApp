# Incremental implementation plan

The POC targets Amazon Nova 2 Sonic in Brazilian Portuguese. Nova is managed by
Bedrock, so there is no GPU provisioning or model download.

## Git workflow

Every stage uses a dedicated branch, small commits, a clean test gate, a
`--no-ff` merge into `main`, and an annotated tag. History is not squashed.

## Stages

1. **Bootstrap (`v0.1.0`, complete):** repository, Go module, commands, build
   targets, and architecture.
2. **AWS Nova foundation (`stage/01-aws-nova`, `v0.2.0`):** least-privilege
   IAM, local Python bridge, PT-BR `carolina` voice, smoke test, and AWS setup.
3. **Go gateway (`stage/02-go-gateway`, `v0.3.0`):** health/providers APIs,
   versioned voice WebSocket, session state, limits, metrics, and fake provider.
4. **Nova end to end (`stage/03-nova-e2e`, `v0.4.0`):** Go bridge adapter,
   native bidirectional audio, transcript mapping, barge-in, tool event mapping,
   and eight-minute session continuation.
5. **MCP notes (original Stage 6, `stage/06-mcp-notes`, `v0.5.0`):** SQLite,
   configurable MCP discovery, create/list/delete tools, idempotency,
   confirmation, and native Nova tool use, validated through the terminal.
6. **macOS app (`stage/04-macos-app`, `v0.6.0`):** shared SwiftUI code,
   microphone capture, voice processing, incremental playback, and UI states.
7. **iOS app (`stage/05-ios-app`, `v0.7.0`):** iPhone target, local network,
   audio route/lifecycle handling, Simulator build, and physical-device test.
8. **Resilience (`stage/07-resilience`, `v0.8.0`):** reconnect, session
   renewal, timeouts, ordering, redaction, and failure tests.
9. **POC release (`stage/08-poc-release`, `v1.0.0-poc`):** PT-BR scenarios,
   latency benchmark, device validation, setup guide, and demo script.

## Current implementation status

- Bootstrap is merged and tagged as `v0.1.0`.
- The former Qwen/RunPod stage was renamed to `stage/01-aws-nova`; its obsolete
  deployment artifacts were removed while preserving the incremental history.
- The Nova bridge, AWS policy, PT-BR configuration, unit tests, and live smoke
  harness are implemented.
- Stage 1 is accepted: IAM simulation returned `allowed`; the live PT-BR
  smoke test captured 172,800 bytes / 3.6 seconds of PCM16 mono at 24 kHz.
  The user authorized progression to Stage 2 after reviewing the AWS panel.
- Stage 1 is merged into `main` and tagged `v0.2.0`.
- Stage 2 is implemented on `stage/02-go-gateway`: protocol v1, HTTP endpoints,
  authenticated WebSocket, bounded sessions, fake provider, cancellation,
  metrics, terminal WAV demo, and concurrency tests. The user confirmed the
  terminal execution and accepted progression to Stage 3. No AWS calls were
  needed for Stage 2.
- Stage 2 is merged into `main` and tagged `v0.3.0`.
- Stage 3 is implemented on `stage/03-nova-e2e`: live continuous audio through
  Go, final/speculative transcripts, native barge-in, stale-frame rejection,
  explicit cancellation, tool-event mapping, and timed history-based renewal.
  Two real AWS sessions passed (normal response and native speech interruption).
  Session renewal was validated with compressed timers against a mock bridge,
  not a real eight-minute soak. The user authorized implementation of the next
  stage; Stage 3 passed its gate, was merged `--no-ff`, and tagged `v0.4.0`.
  Apple microphone/playback validation remains in Stages 4–5.
- The user requested original Stage 6 before either Apple app. Stage identifiers
  and branch names remain stable; future tags follow execution order. No existing
  history or tags are rewritten.
- Stage 6 is implemented on `stage/06-mcp-notes`: real stdio MCP discovery and
  calls, SQLite notes/effect ledger/session audit, native Nova tool configuration
  and result transport, argument validation, host confirmation, bounded async
  execution, and terminal follow-ups/event capture. Live AWS create, retrieval
  in a fresh conversation, durable retry, refusal, confirmed deletion and empty
  post-delete query passed. See [MCP integration](mcp-integration.md).
  The user accepted Stage 6 and authorized its `--no-ff` merge and annotated
  release tag `v0.5.0`, followed by publication to their GitHub repository.

## Next priority: MCP before Apple apps

Execution order is **0 → 1 → 2 → 3 → 6 → 4 → 5 → 7 → 8**.
Stage 3's acceptance/test/merge gate is closed. The MCP branch was created from
updated `main`; neither Apple branch starts before MCP acceptance and merge.

### Integration boundary

The model does not connect directly to an MCP server. The Go orchestrator
initializes a configured local MCP server over stdio, discovers `tools/list`,
and converts allowed tool schemas into Nova `promptStart.toolConfiguration`.
Nova chooses whether to emit `toolUse`; Go validates and executes `tools/call`,
then the Python transport returns the correlated `toolResult` to the same Nova
conversation so it can speak from the actual result. The Python bridge remains
transport-only; business logic, permissions and persistence stay in Go.

Stage 3 alone only mapped tool events. Stage 6 now advertises MCP tools,
executes them, and returns results; `cmd/mcp-notes` is now a working stdio server.

### Incremental tasks and commits

1. `feat(storage): add sqlite migrations and notes repositories`
   - Persist notes and operation lifecycle; unique idempotency keys.
2. `feat(mcp): implement notes server and configurable stdio client`
   - MCP initialization, discovery and calls; bounded payloads, timeouts and
     clean subprocess shutdown. Use a configured allowlist, never a model-supplied
     command. Start with create/list/delete; make the client reusable for other
     configured MCP servers without claiming universal compatibility.
3. `feat(nova): advertise discovered tools and return native tool results`
   - Map MCP names to Nova-compatible names, preserve JSON schemas and tool-use
     IDs, serialize writes with audio, and re-advertise tools on session renewal.
4. `feat(orchestrator): validate tool calls and enforce safe mutations`
   - Validate names/arguments; deduplicate within session and across retries;
     require explicit confirmation before deletion. A pending confirmation is
     not success. Return controlled failures to Nova for every failed tool call.
5. `feat(client): display tool lifecycle and confirmation in terminal`
   - Expose started/result/error/confirmation events without requiring SwiftUI.
     Apple tool UI moves to the later app stages. Preserve voice barge-in while
     tools run; an interrupted response must not silently retry a mutation.
6. `test(mcp): verify discovery decisions actions and spoken results`
   - Unit/integration tests first, then real PT-BR Nova voice scenarios with WAV
     output and local audit evidence. Live calls incur normal Bedrock charges.
7. `docs(mcp): add setup demo and validation report`
   - Document plugging a configured server, tool permissions and observed limits.

### Acceptance gate

- Model autonomously selects tools with automatic tool choice for requests
  requiring external information; ordinary conversation needs no tool call.
- Create a note by voice, query it in a later session, and speak its stored
  information. Use a unique test value to distinguish retrieval from guesswork.
- Delete only after explicit confirmation; refusal leaves the note intact.
- Correlate ASR, tool name/ID, MCP result, SQLite state and final spoken response;
  never report success before the tool result, or fabricate data on tool failure.
- Retry does not duplicate a note; malformed arguments, unknown tools, timeout,
  server loss and interruption return controlled outcomes.
- Go tests with race detector and bridge tests pass; real AWS results are
  documented separately from mocks. Merge/tag only after user acceptance.

Reference: [AWS Nova 2 Sonic tool configuration](https://docs.aws.amazon.com/nova/latest/nova2-userguide/sonic-tool-configuration.html).

## Acceptance targets

- Native, continuous speech-to-speech conversation in PT-BR.
- First useful audio in at most 1.5 seconds at median, measured from Brazil.
- Playback interruption in at most 300 ms.
- At least 95% successful notes operations and no duplicate mutations.
- macOS 27 and iOS 27 builds from the same SwiftUI project.

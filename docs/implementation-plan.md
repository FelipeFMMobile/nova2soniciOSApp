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
5. **macOS app (`stage/04-macos-app`, `v0.5.0`):** shared SwiftUI code,
   microphone capture, voice processing, incremental playback, and UI states.
6. **iOS app (`stage/05-ios-app`, `v0.6.0`):** iPhone target, local network,
   audio route/lifecycle handling, Simulator build, and physical-device test.
7. **MCP notes (`stage/06-mcp-notes`, `v0.7.0`):** SQLite, MCP discovery,
   create/list/delete tools, idempotency, confirmation, and native Nova tool use.
8. **Resilience (`stage/07-resilience`, `v0.8.0`):** reconnect, session
   renewal, timeouts, ordering, redaction, and failure tests.
9. **POC release (`stage/08-poc-release`, `v1.0.0-poc`):** PT-BR scenarios,
   latency benchmark, device validation, setup guide, and demo script.

## Acceptance targets

- Native, continuous speech-to-speech conversation in PT-BR.
- First useful audio in at most 1.5 seconds at median, measured from Brazil.
- Playback interruption in at most 300 ms.
- At least 95% successful notes operations and no duplicate mutations.
- macOS 27 and iOS 27 builds from the same SwiftUI project.


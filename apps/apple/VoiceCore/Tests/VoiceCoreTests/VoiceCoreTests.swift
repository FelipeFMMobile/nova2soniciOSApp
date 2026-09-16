import XCTest
@testable import VoiceCore

final class VoiceCoreTests: XCTestCase {
    func readyState() throws -> ConversationState {
        var state = ConversationState()
        _ = try state.handle(VoiceEvent(type: "session.ready", sessionId: "session-1"))
        _ = try state.handle(VoiceEvent(type: "turn.started", sessionId: "session-1", turnId: "turn-1"))
        return state
    }
    func testClientEncodingMatchesGoContract() throws {
        let event = VoiceEvent(type: "audio.append", sessionId: "s", turnId: "t", sequence: 1, audio: "AAAAAA==", sampleRate: 16000)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: event.encoded()) as? [String: Any])
        XCTAssertEqual(object["version"] as? Int, 1); XCTAssertEqual(object["sequence"] as? Int, 1)
        XCTAssertNil(object["tool"]); XCTAssertNil(object["requestId"])
        XCTAssertEqual(try VoiceEvent.decode(event.encoded()), event)
        XCTAssertThrowsError(try VoiceEvent.decode(Data(#"{"version":2,"type":"session.ready"}"#.utf8)))
    }
    func testCancelledAndStaleAudioNeverReturns() throws {
        var state = try readyState()
        let audio = VoiceEvent(type: "audio.output", sessionId: "session-1", turnId: "turn-1", sequence: 1, audio: "AAAAAA==", sampleRate: 24000)
        XCTAssertEqual(try state.handle(audio), [.play(Data(repeating: 0, count: 4))])
        XCTAssertEqual(try state.handle(VoiceEvent(type: "turn.interrupted", sessionId: "session-1", turnId: "turn-1")), [.clearPlayback])
        XCTAssertEqual(try state.handle(audio), []); XCTAssertEqual(state.rejectedAudio, 1)
        _ = try state.handle(VoiceEvent(type: "turn.started", sessionId: "session-1", turnId: "turn-2"))
        XCTAssertEqual(try state.handle(audio), [])
    }
    func testOrderingFormatAndSessionAreValidated() throws {
        var state = try readyState()
        XCTAssertThrowsError(try state.handle(VoiceEvent(type: "audio.output", sessionId: "session-1", turnId: "turn-1", sequence: 2, audio: "AAAAAA==", sampleRate: 24000)))
        XCTAssertThrowsError(try state.handle(VoiceEvent(type: "audio.output", sessionId: "session-1", turnId: "turn-1", sequence: 1, audio: "AAAA", sampleRate: 24000)))
        XCTAssertThrowsError(try state.handle(VoiceEvent(type: "session.state", sessionId: "another")))
    }
    func testRenewalKeepsHistoryButChangesInput() throws {
        var state = try readyState()
        _ = try state.handle(VoiceEvent(type: "transcript", sessionId: "session-1", turnId: "turn-1", text: "Olá", role: "USER", stage: "FINAL"))
        XCTAssertEqual(try state.handle(VoiceEvent(type: "session.renewed", sessionId: "session-1")), [.clearPlayback, .renewInput])
        XCTAssertEqual(state.transcripts.count, 1); XCTAssertNil(state.outputTurnId)
    }
    func testToolsDoNotClaimSuccessBeforeResult() throws {
        var state = try readyState()
        _ = try state.handle(VoiceEvent(type: "tool.started", sessionId: "session-1", tool: VoiceTool(operationId: "op-1", name: "notes_create")))
        XCTAssertEqual(state.operations.first?.status, "Em execução")
        let pending = try JSONDecoder().decode(JSONValue.self, from: Data(#"{"content":[{"type":"text","text":"{\"status\":\"confirmation_required\"}"}]}"#.utf8))
        _ = try state.handle(VoiceEvent(type: "tool.result", sessionId: "session-1", tool: VoiceTool(operationId: "op-1", name: "notes_create", result: pending)))
        XCTAssertTrue(try XCTUnwrap(state.operations.first).needsConfirmation)
        let failed: JSONValue = .object(["isError": .bool(true)])
        _ = try state.handle(VoiceEvent(type: "tool.result", sessionId: "session-1", tool: VoiceTool(operationId: "op-1", name: "notes_create", result: failed)))
        XCTAssertEqual(state.operations.first?.status, "Falhou ou resultado incerto")
    }
    func testInputFramesStayBoundedAndContiguous() throws {
        var frames = AudioFrames()
        XCTAssertTrue(try frames.append(Data(repeating: 0, count: 512)).isEmpty)
        XCTAssertEqual(try frames.append(Data(repeating: 1, count: 1536)).map(\.count), [1024, 1024])
        XCTAssertThrowsError(try frames.append(Data(repeating: 0, count: 3)))
        frames.reset(); XCTAssertTrue(try frames.append(Data(repeating: 0, count: 2)).isEmpty)
    }
    func testEndpointDoesNotAllowEmbeddedSecrets() throws {
        XCTAssertNoThrow(try GatewayConfiguration.validate("ws://127.0.0.1:8080/v1/voice"))
        XCTAssertNoThrow(try GatewayConfiguration.validate("wss://gateway.example/v1/voice"))
        for url in ["https://example/v1/voice", "ws://token@example/v1/voice", "ws://example/v1/voice?token=secret", "ws://example/other"] {
            XCTAssertThrowsError(try GatewayConfiguration.validate(url))
        }
    }
}

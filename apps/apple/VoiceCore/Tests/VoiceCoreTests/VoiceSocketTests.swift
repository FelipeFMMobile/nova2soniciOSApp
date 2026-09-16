import XCTest
@testable import VoiceCore

final class VoiceSocketTests: XCTestCase {
    @MainActor func testDisconnectedSendAndInvalidConfiguration() throws {
        let client = VoiceSocket()
        XCTAssertThrowsError(try client.send(VoiceEvent(type: "session.stop")))
        XCTAssertThrowsError(try client.connect(url: "ws://localhost/v1/voice", token: "bad\r\nheader", provider: "fake", requestId: "test"))
        XCTAssertThrowsError(try client.connect(url: "ws://localhost/v1/voice", token: "", provider: "qwen", requestId: "test"))
        client.disconnect(); client.disconnect()
    }

    /// Opt in with STS_APPLE_E2E_URL. This connects only to the local fake gateway.
    @MainActor func testFakeGatewayLifecycleAndReconnect() async throws {
        guard let url = ProcessInfo.processInfo.environment["STS_APPLE_E2E_URL"] else { throw XCTSkip("Local fake gateway not requested") }
        let client = VoiceSocket()
        defer { client.disconnect() }
        for _ in 0..<2 {
            let stopped = expectation(description: "Session stopped")
            var state = ConversationState()
            var heardAudio = false
            var transcript = false
            var stoppedReceived = false
            client.onFailure = { error in XCTFail(error.localizedDescription); if !stoppedReceived { stoppedReceived = true; stopped.fulfill() } }
            client.onEvent = { event in
                do {
                    let effects = try state.handle(event)
                    if effects.contains(.ready) {
                        try client.send(VoiceEvent(type: "audio.append", sessionId: state.sessionId, turnId: "apple-test", sequence: 1, audio: "AAA=", sampleRate: 16000))
                        try client.send(VoiceEvent(type: "turn.commit", sessionId: state.sessionId, turnId: "apple-test"))
                    }
                    if event.type == "audio.output" { heardAudio = true }
                    if event.type == "transcript" { transcript = true }
                    if event.type == "turn.completed" { try client.send(VoiceEvent(type: "session.stop", sessionId: state.sessionId)) }
                    if effects.contains(.stopped), !stoppedReceived { stoppedReceived = true; stopped.fulfill() }
                } catch { XCTFail(String(describing: error)); if !stoppedReceived { stoppedReceived = true; stopped.fulfill() } }
            }
            try client.connect(url: url, token: "apple-test-token", provider: "fake", requestId: UUID().uuidString)
            await fulfillment(of: [stopped], timeout: 10)
            client.disconnect()
            XCTAssertTrue(heardAudio); XCTAssertTrue(transcript)
        }
    }
}

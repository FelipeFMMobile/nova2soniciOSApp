import XCTest
@testable import VoiceCore

final class VoiceSocketTests: XCTestCase {
    @VoiceNetworkActor func testDisconnectedSendAndInvalidConfiguration() async throws {
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
        defer { Task { await client.disconnect() } }
        for _ in 0..<2 {
            let stopped = expectation(description: "Session stopped")
            var state = ConversationState()
            var heardAudio = false
            var transcript = false
            var stoppedReceived = false
            await client.configure(onEvent: { event in
                do {
                    let effects = try state.handle(event)
                    if effects.contains(.ready) {
                        let session = state.sessionId
                        Task {
                            do {
                                let pump = VoiceInputPump(session: try XCTUnwrap(session), turn: "apple-test")
                                try await pump.send(Data(repeating: 0, count: 16000), socket: client)
                                let stats = await pump.stats
                                XCTAssertEqual(stats.buffers, 1); XCTAssertEqual(stats.bytes, 16000)
                                try await client.send(VoiceEvent(type: "turn.commit", sessionId: session, turnId: "apple-test"))
                            } catch { XCTFail(String(describing: error)) }
                        }
                    }
                    if event.type == "audio.output" { heardAudio = true }
                    if event.type == "transcript" { transcript = true }
                    if event.type == "turn.completed" {
                        let session = state.sessionId
                        Task {
                            do { try await client.send(VoiceEvent(type: "session.stop", sessionId: session)) }
                            catch { XCTFail(String(describing: error)) }
                        }
                    }
                    if effects.contains(.stopped), !stoppedReceived { stoppedReceived = true; stopped.fulfill() }
                } catch { XCTFail(String(describing: error)); if !stoppedReceived { stoppedReceived = true; stopped.fulfill() } }
            }, onFailure: { error in XCTFail(error.localizedDescription); if !stoppedReceived { stoppedReceived = true; stopped.fulfill() } })
            try await client.connect(url: url, token: "apple-test-token", provider: "fake", requestId: UUID().uuidString)
            await fulfillment(of: [stopped], timeout: 10)
            await client.disconnect()
            XCTAssertTrue(heardAudio); XCTAssertTrue(transcript)
        }
    }
}

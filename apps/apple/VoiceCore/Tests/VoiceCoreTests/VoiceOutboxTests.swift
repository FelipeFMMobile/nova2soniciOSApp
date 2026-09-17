import XCTest
@testable import VoiceCore

final class VoiceOutboxTests: XCTestCase {
    @MainActor func testShortStallResumesWithoutLosingOrReorderingAudio() async throws {
        let queue = VoiceOutbox(capacity: 1)
        try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 1))
        var resumed = false
        let producer = Task {
            try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 2))
            resumed = true
        }
        defer { producer.cancel(); queue.close() }
        try await Task.sleep(for: .milliseconds(30))
        XCTAssertFalse(resumed)
        var reader = queue.stream.makeAsyncIterator()
        let first = await reader.next()
        XCTAssertEqual(first?.sequence, 1)
        XCTAssertTrue(queue.audioFull, "Reading does not free capacity until send completes")
        queue.sent()
        try await producer.value
        XCTAssertTrue(resumed)
        let second = await reader.next()
        XCTAssertEqual(second?.sequence, 2)
    }

    @MainActor func testPersistentStallHasSpecificTimeout() async throws {
        let queue = VoiceOutbox(capacity: 1)
        defer { queue.close() }
        try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 1))
        do {
            try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 2), timeout: .milliseconds(20))
            XCTFail("Expected congestion timeout")
        } catch { XCTAssertEqual(error as? VoiceFailure, .sendBackpressure) }
    }

    @MainActor func testControlSlotsRemainAvailableWhenAudioIsFull() async throws {
        let queue = VoiceOutbox(capacity: 1)
        defer { queue.close() }
        try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 1))
        try queue.enqueueControl(VoiceEvent(type: "session.stop"))
        try queue.enqueueControl(VoiceEvent(type: "turn.commit"))
        XCTAssertThrowsError(try queue.enqueueControl(VoiceEvent(type: "session.stop"))) {
            XCTAssertEqual($0 as? VoiceFailure, .sendBackpressure)
        }
        var reader = queue.stream.makeAsyncIterator()
        let first = await reader.next(); let second = await reader.next(); let third = await reader.next()
        XCTAssertEqual([first?.type, second?.type, third?.type], ["audio.append", "session.stop", "turn.commit"])
    }

    @MainActor func testWaitingProducerCanBeCancelled() async throws {
        let queue = VoiceOutbox(capacity: 1)
        defer { queue.close() }
        try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 1))
        let producer = Task { try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 2)) }
        try await Task.sleep(for: .milliseconds(20))
        producer.cancel()
        do { try await producer.value; XCTFail("Expected cancellation") }
        catch { XCTAssertTrue(error is CancellationError) }
    }

    @MainActor func testCloseReleasesWaitingProducerAndRejectsStaleFrames() async throws {
        let queue = VoiceOutbox(capacity: 1)
        try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 1))
        let producer = Task { try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 2)) }
        try await Task.sleep(for: .milliseconds(20))
        queue.close()
        do { try await producer.value; XCTFail("Expected disconnected queue") }
        catch { XCTAssertEqual(error as? VoiceFailure, .disconnected) }
        queue.sent()
        do { try await queue.enqueueAudio(VoiceEvent(type: "audio.append", sequence: 3)); XCTFail("Expected disconnected queue") }
        catch { XCTAssertEqual(error as? VoiceFailure, .disconnected) }
    }
}

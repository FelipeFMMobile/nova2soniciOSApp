import Foundation

/// One audio producer, one writer. Capacity includes the frame currently in flight.
/// Two reserved control slots allow session.stop even while audio is congested.
@MainActor final class VoiceOutbox {
    let stream: AsyncStream<VoiceEvent>
    private let continuation: AsyncStream<VoiceEvent>.Continuation
    private let capacity: Int
    private var pending = 0
    private var closed = false
    var audioFull: Bool { pending >= capacity }

    init(capacity: Int = 16) {
        precondition(capacity > 0)
        self.capacity = capacity
        // Admission below bounds the stream; it cannot accumulate unbounded events.
        let channel = AsyncStream<VoiceEvent>.makeStream()
        stream = channel.stream; continuation = channel.continuation
    }

    func enqueueControl(_ event: VoiceEvent) throws {
        guard !closed else { throw VoiceFailure.disconnected }
        guard pending < capacity + 2 else { throw VoiceFailure.sendBackpressure }
        admit(event)
    }

    func enqueueAudio(_ event: VoiceEvent, timeout: Duration = .seconds(1)) async throws {
        let clock = ContinuousClock()
        let deadline = clock.now.advanced(by: timeout)
        while audioFull {
            try Task.checkCancellation()
            guard !closed else { throw VoiceFailure.disconnected }
            guard clock.now < deadline else { throw VoiceFailure.sendBackpressure }
            // Suspend the producer (not the UI/audio render thread) until space opens.
            try await Task.sleep(for: .milliseconds(10))
        }
        try Task.checkCancellation()
        guard !closed else { throw VoiceFailure.disconnected }
        admit(event)
    }

    private func admit(_ event: VoiceEvent) {
        pending += 1
        continuation.yield(event)
    }
    func sent() { pending = max(0, pending - 1) }
    func close() { closed = true; continuation.finish() }
}

import Foundation

public struct VoiceInputStats: Sendable {
    public var buffers = 0
    public var bytes = 0
    public var level = 0.0
    public var lastCapture: Date?
    public var lastSignal: Date?
}

@VoiceNetworkActor public final class VoiceInputPump {
    private let session: String
    private var turn: String
    private var sequence: UInt64 = 0
    private var frames = AudioFrames()
    public private(set) var stats = VoiceInputStats()

    public nonisolated init(session: String, turn: String) {
        self.session = session; self.turn = turn
    }
    public func renew(turn: String) {
        self.turn = turn; sequence = 0; frames.reset()
    }
    public func run(_ stream: AsyncStream<Data>, socket: VoiceSocket) async throws {
        for await data in stream {
            try Task.checkCancellation()
            try await send(data, socket: socket)
        }
    }
    public func send(_ data: Data, socket: VoiceSocket) async throws {
        stats.buffers += 1; stats.bytes += data.count
        stats.level = VoiceDiagnostics.level(data); stats.lastCapture = Date()
        if stats.level > 0.001 { stats.lastSignal = Date() }
        let currentTurn = turn
        for frame in try frames.append(data) {
            try Task.checkCancellation()
            guard turn == currentTurn else { return }
            sequence += 1
            try await socket.sendAudio(VoiceEvent(type: "audio.append", sessionId: session, turnId: currentTurn,
                                                  sequence: sequence, audio: frame.base64EncodedString(), sampleRate: 16000))
        }
    }
}

import Foundation

public struct AudioFrames: Sendable {
    public static let bytesPerFrame = 1024 // 512 samples / 32 ms, mono PCM16 16 kHz.
    private var pending = Data()
    public init() {}
    public mutating func append(_ pcm: Data) throws -> [Data] {
        guard pcm.count % 2 == 0, pcm.count <= 32000 else { throw VoiceFailure.audioFormat }
        pending.append(pcm)
        var frames: [Data] = []
        while pending.count >= Self.bytesPerFrame {
            frames.append(Data(pending.prefix(Self.bytesPerFrame))); pending.removeFirst(Self.bytesPerFrame)
        }
        return frames
    }
    public mutating func reset() { pending.removeAll(keepingCapacity: true) }
}

public enum GatewayConfiguration {
    public static func validate(_ raw: String) throws -> URL {
        guard raw.utf8.count <= 1024, let url = URL(string: raw.trimmingCharacters(in: .whitespacesAndNewlines)),
              let scheme = url.scheme, ["ws", "wss"].contains(scheme),
              let host = url.host, !host.isEmpty, url.path == "/v1/voice",
              url.user == nil, url.password == nil, url.query == nil, url.fragment == nil else { throw VoiceFailure.invalidConfiguration }
        return url
    }
}

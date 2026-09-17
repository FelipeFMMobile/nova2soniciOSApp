import Foundation

public enum VoiceDiagnostics {
    /// RMS of converted PCM16, normalized to 0...1. Never retains audio.
    public static func level(_ pcm: Data) -> Double {
        guard pcm.count >= 2 else { return 0 }
        return pcm.withUnsafeBytes { bytes in
            var sum = 0.0
            for index in 0..<(pcm.count / 2) {
                let sample = Double(Int16(littleEndian: bytes.loadUnaligned(fromByteOffset: index * 2, as: Int16.self))) / 32768
                sum += sample * sample
            }
            return sqrt(sum / Double(pcm.count / 2))
        }
    }

    /// Metadata only: no tokens, PCM, transcript, tool arguments or error messages.
    public static func summary(_ event: VoiceEvent) -> String {
        "type=\(event.type) sequence=\(event.sequence ?? 0) audioBase64Bytes=\(event.audio?.utf8.count ?? 0)"
    }
}

import AVFoundation

/// Transport rates belong at the edges, not on the Voice Processor's duplex I/O.
public enum VoiceGraphFormat {
    public static func output(microphone: AVAudioFormat?, hardware: AVAudioFormat) throws -> AVAudioFormat {
        let format = microphone ?? hardware
        guard format.sampleRate.isFinite, format.sampleRate > 0, format.channelCount > 0,
              format.commonFormat == .pcmFormatFloat32 else { throw VoiceFailure.audioFormat }
        return format
    }
}

import AVFoundation

/// Call only on a single audio-tap queue. Copies PCM so no render buffer escapes.
public final class PCMConverter: @unchecked Sendable {
    private let converter: AVAudioConverter
    private let output: AVAudioFormat
    public init(input: AVAudioFormat) throws {
        guard input.sampleRate > 0, input.channelCount > 0,
              let output = AVAudioFormat(commonFormat: .pcmFormatInt16, sampleRate: 16000, channels: 1, interleaved: true),
              let converter = AVAudioConverter(from: input, to: output) else { throw VoiceFailure.audioFormat }
        self.output = output; self.converter = converter
    }
    public func convert(_ input: AVAudioPCMBuffer) throws -> Data {
        guard input.format == converter.inputFormat, input.frameLength <= 48000 else { throw VoiceFailure.audioFormat }
        let capacity = AVAudioFrameCount(ceil(Double(input.frameLength) * 16000 / input.format.sampleRate) + 64)
        guard let buffer = AVAudioPCMBuffer(pcmFormat: output, frameCapacity: capacity) else { throw VoiceFailure.audioFormat }
        var consumed = false
        var error: NSError?
        let status = converter.convert(to: buffer, error: &error) { _, state in
            if consumed { state.pointee = .noDataNow; return nil }
            consumed = true; state.pointee = .haveData; return input
        }
        guard status != .error, error == nil else { throw VoiceFailure.audioFormat }
        guard buffer.frameLength > 0, let bytes = buffer.audioBufferList.pointee.mBuffers.mData else { return Data() }
        return Data(bytes: bytes, count: Int(buffer.frameLength) * 2)
    }
}

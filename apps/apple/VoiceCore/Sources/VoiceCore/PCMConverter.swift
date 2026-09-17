import AVFoundation
import OSLog

/// Call only on a single audio-tap queue. Copies PCM so no render buffer escapes.
public final class PCMConverter: @unchecked Sendable {
    private let converter: AVAudioConverter
    private let output: AVAudioFormat
    private let log = Logger(subsystem: "com.sts.NovaVoice", category: "Audio")
    private var diagnosticFrames: UInt64 = 0
    private var nextDiagnosticFrame: UInt64 = 0
    public init(input: AVAudioFormat) throws {
        guard input.sampleRate > 0, input.channelCount > 0,
              let output = AVAudioFormat(commonFormat: .pcmFormatInt16, sampleRate: 16000, channels: 1, interleaved: true),
              let converter = AVAudioConverter(from: input, to: output) else { throw VoiceFailure.audioFormat }
        self.output = output; self.converter = converter
        // Select the microphone explicitly; do not let a multichannel layout
        // choose an implicit mono downmix (VoiceProcessing can expose extra channels).
        converter.downmix = false
        converter.channelMap = [0]
    }
    public func convert(_ input: AVAudioPCMBuffer) throws -> Data {
        guard input.format == converter.inputFormat, input.frameLength <= 48000 else { throw VoiceFailure.audioFormat }
        let reportLevels = input.frameLength > 0 && diagnosticFrames >= nextDiagnosticFrame
        diagnosticFrames += UInt64(input.frameLength)
        if reportLevels {
            nextDiagnosticFrame = diagnosticFrames + UInt64(input.format.sampleRate)
            let levels = VoiceDiagnostics.channelLevels(input).map { String(format: "%.8f", $0) }.joined(separator: ",")
            log.info("RAW channelsRMS=[\(levels, privacy: .public)] selectedChannel=0 frames=\(input.frameLength)")
        }
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
        let pcm = Data(bytes: bytes, count: Int(buffer.frameLength) * 2)
        if reportLevels {
            let level = String(format: "%.8f", VoiceDiagnostics.level(pcm))
            log.info("CONVERTED selectedChannel=0 rms=\(level, privacy: .public) bytes=\(pcm.count)")
        }
        return pcm
    }
}

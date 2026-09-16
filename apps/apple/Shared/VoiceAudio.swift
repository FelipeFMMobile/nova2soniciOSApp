import AVFoundation
import VoiceCore

@MainActor final class VoiceAudio {
    private let engine = AVAudioEngine()
    private let player = AVAudioPlayerNode()
    private let playbackFormat = AVAudioFormat(standardFormatWithSampleRate: 24000, channels: 1)!
    private var tapInstalled = false
    private var attached = false
    #if os(iOS)
    private var sessionActive = false
    #endif
    private var capture: AsyncStream<Data>.Continuation?
    private var pending = Data()
    private var scheduled = 0
    private var playbackGeneration = 0
    private var failure: (@MainActor (VoiceFailure) -> Void)?
    var onPlayingChanged: (@MainActor (Bool) -> Void)?

    static func requestPermission() async -> Bool {
        #if os(iOS)
        return await withCheckedContinuation { continuation in
            AVAudioApplication.requestRecordPermission { allowed in continuation.resume(returning: allowed) }
        }
        #else
        return await AVCaptureDevice.requestAccess(for: .audio)
        #endif
    }

    func start(microphone: Bool, onFailure: @escaping @MainActor (VoiceFailure) -> Void) throws -> AsyncStream<Data> {
        stop(); failure = onFailure
        #if os(iOS)
        let audioSession = AVAudioSession.sharedInstance()
        try audioSession.setCategory(microphone ? .playAndRecord : .playback, mode: microphone ? .voiceChat : .default,
                                     options: microphone ? [.defaultToSpeaker, .allowBluetoothHFP] : [])
        try audioSession.setActive(true)
        sessionActive = true
        #endif
        if !attached { engine.attach(player); attached = true }
        if microphone { try engine.inputNode.setVoiceProcessingEnabled(true) }
        if #available(macOS 27, iOS 27, *) { try engine.connectNode(player, to: engine.mainMixerNode, format: playbackFormat) }
        else { engine.connect(player, to: engine.mainMixerNode, format: playbackFormat) }
        let channel = AsyncStream<Data>.makeStream(bufferingPolicy: .bufferingOldest(16))
        capture = channel.continuation
        if microphone {
            let input = engine.inputNode
            let format = input.outputFormat(forBus: 0)
            guard format.sampleRate > 0, format.channelCount > 0 else { throw VoiceFailure.audioFormat }
            let converter = try PCMConverter(input: format)
            let tap: AVAudioNodeTapBlock = { buffer, _ in
                do {
                    let pcm = try converter.convert(buffer)
                    if pcm.isEmpty { return }
                    if case .dropped = channel.continuation.yield(pcm) {
                        channel.continuation.finish()
                        Task { @MainActor in onFailure(.backpressure) }
                    }
                } catch {
                    channel.continuation.finish()
                    Task { @MainActor in onFailure(.audioFormat) }
                }
            }
            if #available(macOS 27, iOS 27, *) {
                try input.__installTap(onBus: 0, bufferSize: 1024, format: format, error: (), block: tap)
            }
            else { input.installTap(onBus: 0, bufferSize: 1024, format: format, block: tap) }
            tapInstalled = true
        }
        engine.prepare(); try engine.start()
        return channel.stream
    }

    func enqueue(_ pcm: Data) throws {
        // Nova generates faster than playback. Bound storage to 30 s, but schedule only ~256 ms.
        guard pending.count + pcm.count <= 24000 * 2 * 30 else { throw VoiceFailure.backpressure }
        pending.append(pcm); pump()
    }
    func clearPlayback() {
        playbackGeneration += 1; player.stop(); pending.removeAll(keepingCapacity: false); scheduled = 0
        onPlayingChanged?(false)
    }
    private func pump() {
        guard engine.isRunning else { return }
        while scheduled < 6 && !pending.isEmpty {
            let count = min(2048, pending.count)
            let data = pending.prefix(count)
            pending.removeFirst(count)
            guard let buffer = AVAudioPCMBuffer(pcmFormat: playbackFormat, frameCapacity: AVAudioFrameCount(count / 2)),
                  let channel = buffer.floatChannelData?[0] else { failure?(.audioFormat); return }
            buffer.frameLength = AVAudioFrameCount(count / 2)
            data.withUnsafeBytes { raw in
                for i in 0..<(count / 2) { channel[i] = Float(Int16(littleEndian: raw.loadUnaligned(fromByteOffset: i * 2, as: Int16.self))) / 32768 }
            }
            let epoch = playbackGeneration
            scheduled += 1
            player.scheduleBuffer(buffer, completionCallbackType: .dataPlayedBack) { [weak self] _ in
                Task { @MainActor [weak self] in
                    guard let self, self.playbackGeneration == epoch else { return }
                    self.scheduled -= 1; self.pump()
                    if self.scheduled == 0 && self.pending.isEmpty { self.onPlayingChanged?(false) }
                }
            }
        }
        if scheduled > 0 {
            if !player.isPlaying {
                do {
                    if #available(macOS 27, iOS 27, *) { try player.playAudio() }
                    else { player.play() }
                } catch { failure?(.audioFormat); return }
            }
            onPlayingChanged?(true)
        }
    }
    func stop() {
        if tapInstalled { engine.inputNode.removeTap(onBus: 0); tapInstalled = false }
        capture?.finish(); capture = nil
        clearPlayback(); engine.stop(); failure = nil
        #if os(iOS)
        if sessionActive {
            sessionActive = false
            try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        }
        #endif
    }
}

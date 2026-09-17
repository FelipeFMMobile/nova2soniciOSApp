import SwiftUI
import AVFoundation
import VoiceCore
import OSLog

@MainActor final class ConversationModel: ObservableObject {
    @Published var conversation = ConversationState()
    @Published var url = Bundle.main.object(forInfoDictionaryKey: "STSGatewayURL") as? String ?? "ws://127.0.0.1:8080/v1/voice"
    @Published var token = Bundle.main.object(forInfoDictionaryKey: "STSGatewayToken") as? String ?? "local"
    @Published var provider = Bundle.main.object(forInfoDictionaryKey: "STSProvider") as? String ?? "nova"
    @Published private(set) var active = false
    @Published private(set) var stopping = false
    @Published private(set) var playing = false
    @Published private(set) var errorMessage: String?
    @Published private(set) var requestId: String?
    @Published private(set) var inputLevel = 0.0
    @Published private(set) var capturedBuffers = 0
    @Published private(set) var capturedBytes = 0
    @Published private(set) var sentFrames = 0
    @Published private(set) var audioWarning: String?
    private let audioLog = Logger(subsystem: "com.sts.NovaVoice", category: "Audio")
    private var diagnosticsTask: Task<Void, Never>?
    private var inputPump: VoiceInputPump?
    private var cleanupTask: Task<Void, Never>?
    private let socket = VoiceSocket()
    private let audio = VoiceAudio()
    private var inputTask: Task<Void, Never>?
    private var startTask: Task<Void, Never>?
    private var stopTask: Task<Void, Never>?
    private var inputTurn = UUID().uuidString
    private var epoch = 0
    private var observers: [NSObjectProtocol] = []
    private var testMode = false

    init() {
        #if DEBUG
        testMode = ProcessInfo.processInfo.arguments.contains("--ui-testing")
        if testMode {
            provider = "fake"
            url = ProcessInfo.processInfo.environment["STS_TEST_URL"] ?? "ws://127.0.0.1:18080/v1/voice"
            token = ProcessInfo.processInfo.environment["STS_TEST_TOKEN"] ?? "apple-test-token"
        }
        #endif
        audio.setPlayingCallback { [weak self] value in
            guard let self else { return }; self.playing = self.active && value
        }
        #if os(iOS)
        for name in [AVAudioSession.didBecomeInactiveNotification, AVAudioSession.routeChangeNotification] {
            observers.append(NotificationCenter.default.addObserver(forName: name, object: nil, queue: .main) { [weak self] notification in
                let interrupted = name == AVAudioSession.didBecomeInactiveNotification
                let unplugged = name == AVAudioSession.routeChangeNotification &&
                    (notification.userInfo?[AVAudioSessionRouteChangeReasonKey] as? UInt) == AVAudioSession.RouteChangeReason.oldDeviceUnavailable.rawValue
                if interrupted || unplugged { Task { @MainActor [weak self] in self?.stop() } }
            })
        }
        #endif
    }

    var stateLabel: String {
        if stopping { return "Encerrando" }
        if conversation.state == "connecting" { return "Conectando" }
        if !active { return "Desconectado" }
        if playing || conversation.state == "responding" { return "Respondendo" }
        return provider == "fake" ? "Demonstração simulada" : "Ouvindo"
    }

    func start() {
        guard !active else { return }
        errorMessage = nil
        inputLevel = 0; capturedBuffers = 0; capturedBytes = 0; sentFrames = 0
        audioWarning = nil
        do { _ = try GatewayConfiguration.validate(url) } catch { fail(.invalidConfiguration); return }
        epoch += 1; let generation = epoch
        active = true; stopping = false
        conversation = ConversationState(); conversation.connecting(); resetInput()
        let request = UUID().uuidString; requestId = request
        startTask = Task { [weak self] in
            guard let self else { return }
            await self.cleanupTask?.value
            guard !Task.isCancelled, self.epoch == generation else { return }
            if self.provider == "nova", !(await VoiceAudio.requestPermission()) {
                if self.epoch == generation { self.fail(.microphoneDenied) }; return
            }
            guard !Task.isCancelled, self.epoch == generation else { return }
            do {
                await self.socket.configure(onEvent: { [weak self] event in
                    guard self?.epoch == generation else { return }; self?.receive(event)
                }, onFailure: { [weak self] error in
                    guard self?.epoch == generation else { return }; self?.fail(error)
                })
                guard !Task.isCancelled, self.epoch == generation else { return }
                try await self.socket.connect(url: self.url, token: self.token, provider: self.provider, requestId: request)
            } catch {
                guard !Task.isCancelled, self.epoch == generation else { return }
                self.fail((error as? VoiceFailure) ?? .disconnected)
            }
        }
    }

    func stop() {
        guard active, !stopping else { return }
        stopping = true; startTask?.cancel(); startTask = nil
        inputTask?.cancel(); inputTask = nil; audio.stop()
        diagnosticsTask?.cancel(); diagnosticsTask = nil; inputLevel = 0
        guard let session = conversation.sessionId else { finish(); return }
        let generation = epoch
        stopTask = Task { [weak self] in
            guard let self else { return }
            do {
                try await self.socket.send(VoiceEvent(type: "session.stop", sessionId: session))
                try await Task.sleep(for: .seconds(2))
            } catch {}
            guard !Task.isCancelled, self.epoch == generation else { return }
            self.finish()
        }
    }
    func shutdown() { finish() }
    private func finish() {
        epoch += 1; active = false; stopping = false; playing = false
        startTask?.cancel(); startTask = nil; stopTask?.cancel(); stopTask = nil
        inputTask?.cancel(); inputTask = nil
        diagnosticsTask?.cancel(); diagnosticsTask = nil; inputLevel = 0
        audio.stop(); conversation.disconnect(); inputPump = nil
        let socket = socket
        cleanupTask = Task { await socket.disconnect() }
    }
    private func fail(_ error: VoiceFailure) { errorMessage = error.localizedDescription; finish() }
    private func resetInput() {
        inputTurn = UUID().uuidString
        if let inputPump {
            let turn = inputTurn
            Task { await inputPump.renew(turn: turn) }
        }
    }

    private func receive(_ event: VoiceEvent) {
        guard active else { return }
        do {
            let effects = try conversation.handle(event)
            for effect in effects {
                switch effect {
                case .clearPlayback: audio.clearPlayback()
                case .play(let data): if !testMode { try audio.enqueue(data) }
                case .renewInput: resetInput()
                case .stopped: finish()
                case .ready: beginAudio()
                }
            }
        } catch { fail((error as? VoiceFailure) ?? .invalidEvent) }
    }
    private func beginAudio() {
        let generation = epoch
        guard let session = conversation.sessionId else { fail(.disconnected); return }
        let pump = VoiceInputPump(session: session, turn: inputTurn)
        inputPump = pump
        inputTask = Task { [weak self] in
            guard let self else { return }
            do {
                if self.provider == "fake" {
                    if !self.testMode {
                        _ = try await self.audio.start(microphone: false) { [weak self] error in
                            guard self?.epoch == generation else { return }; self?.fail(error)
                        }
                    }
                    guard !Task.isCancelled, self.epoch == generation else { return }
                    try await pump.send(Data(repeating: 0, count: 16000), socket: self.socket)
                    guard !Task.isCancelled, self.epoch == generation else { return }
                    try await self.socket.send(VoiceEvent(type: "turn.commit", sessionId: session, turnId: self.inputTurn))
                    return
                }
                let stream = try await self.audio.start(microphone: true) { [weak self] error in
                    guard self?.epoch == generation else { return }; self?.fail(error)
                }
                guard !Task.isCancelled, self.epoch == generation else { return }
                self.audioLog.info("Audio engine started; PCM/network executor=VoiceNetworkActor UI sampling=5Hz")
                self.beginDiagnostics(pump: pump, generation: generation)
                try await pump.run(stream, socket: self.socket)
            } catch {
                guard !Task.isCancelled, self.epoch == generation else { return }
                self.fail((error as? VoiceFailure) ?? .audioFormat)
            }
        }
    }
    private func beginDiagnostics(pump: VoiceInputPump, generation: Int) {
        let started = Date()
        diagnosticsTask = Task { [weak self] in
            var ticks = 0
            while !Task.isCancelled {
                do { try await Task.sleep(for: .milliseconds(200)) } catch { return }
                let stats = await pump.stats
                guard let self, self.epoch == generation, !self.stopping else { return }
                let sent = await self.socket.audioFramesSent
                guard !Task.isCancelled, self.epoch == generation else { return }
                self.capturedBuffers = stats.buffers; self.capturedBytes = stats.bytes
                self.sentFrames = sent; self.inputLevel = stats.level
                let now = Date()
                let stalled = now.timeIntervalSince(stats.lastCapture ?? started) >= 5
                if stalled { self.inputLevel = 0 }
                self.audioWarning = stalled ? "Nenhum áudio convertido recebido há 5 s. Confira o dispositivo de entrada."
                    : (now.timeIntervalSince(stats.lastSignal ?? started) >= 10 ? "Áudio chegando, mas nível muito baixo há 10 s. Confira microfone, volume e modo de voz do sistema." : nil)
                ticks += 1
                if ticks % 5 == 0 {
                    self.audioLog.info("PCM buffers=\(self.capturedBuffers) bytes=\(self.capturedBytes) sentFrames=\(self.sentFrames) rms=\(self.inputLevel) stalled=\(stalled)")
                }
            }
        }
    }
}

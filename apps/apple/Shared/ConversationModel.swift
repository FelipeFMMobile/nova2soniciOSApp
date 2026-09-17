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
    private var lastCapture: Date?
    private var lastSignal: Date?
    private let socket = VoiceSocket()
    private let audio = VoiceAudio()
    private var inputTask: Task<Void, Never>?
    private var startTask: Task<Void, Never>?
    private var stopTask: Task<Void, Never>?
    private var frames = AudioFrames()
    private var inputTurn = UUID().uuidString
    private var sequence: UInt64 = 0
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
        socket.onEvent = { [weak self] event in self?.receive(event) }
        socket.onFailure = { [weak self] error in self?.fail(error) }
        socket.onSent = { [weak self] event in
            if event.type == "audio.append" { self?.sentFrames += 1 }
        }
        audio.onPlayingChanged = { [weak self] value in self?.playing = value }
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
        audioWarning = nil; lastCapture = nil; lastSignal = nil
        do { _ = try GatewayConfiguration.validate(url) } catch { fail(.invalidConfiguration); return }
        epoch += 1; let generation = epoch
        active = true; stopping = false
        conversation = ConversationState(); conversation.connecting(); resetInput()
        let request = UUID().uuidString; requestId = request
        startTask = Task { [weak self] in
            guard let self else { return }
            if self.provider == "nova", !(await VoiceAudio.requestPermission()) {
                if self.epoch == generation { self.fail(.microphoneDenied) }; return
            }
            guard !Task.isCancelled, self.epoch == generation else { return }
            do { try self.socket.connect(url: self.url, token: self.token, provider: self.provider, requestId: request) }
            catch { self.fail((error as? VoiceFailure) ?? .disconnected) }
        }
    }

    func stop() {
        guard active, !stopping else { return }
        stopping = true; startTask?.cancel(); startTask = nil
        inputTask?.cancel(); inputTask = nil; audio.stop()
        diagnosticsTask?.cancel(); diagnosticsTask = nil; inputLevel = 0
        guard let session = conversation.sessionId else { finish(); return }
        do { try socket.send(VoiceEvent(type: "session.stop", sessionId: session)) }
        catch { finish(); return }
        stopTask = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(2)); self?.finish() } catch {}
        }
    }
    func shutdown() { finish() }
    private func finish() {
        epoch += 1; active = false; stopping = false; playing = false
        startTask?.cancel(); startTask = nil; stopTask?.cancel(); stopTask = nil
        inputTask?.cancel(); inputTask = nil
        diagnosticsTask?.cancel(); diagnosticsTask = nil; inputLevel = 0
        audio.stop(); socket.disconnect(); conversation.disconnect(); frames.reset()
    }
    private func fail(_ error: VoiceFailure) { errorMessage = error.localizedDescription; finish() }
    private func resetInput() { inputTurn = UUID().uuidString; sequence = 0; frames.reset() }

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
                case .ready: try beginAudio()
                }
            }
        } catch { fail((error as? VoiceFailure) ?? .invalidEvent) }
    }
    private func beginAudio() throws {
        let generation = epoch
        if provider == "fake" {
            if !testMode { _ = try audio.start(microphone: false) { [weak self] error in self?.fail(error) } }
            inputTask = Task { [weak self] in
                guard let self else { return }
                do {
                    try await self.sendPCM(Data(repeating: 0, count: 16000))
                    guard !Task.isCancelled, self.epoch == generation else { return }
                    try self.socket.send(VoiceEvent(type: "turn.commit", sessionId: self.conversation.sessionId, turnId: self.inputTurn))
                } catch {
                    guard !Task.isCancelled, self.epoch == generation else { return }
                    self.fail((error as? VoiceFailure) ?? .disconnected)
                }
            }
            return
        }
        let stream = try audio.start(microphone: true) { [weak self] error in
            guard self?.epoch == generation else { return }; self?.fail(error)
        }
        audioLog.info("Audio engine started; waiting for converted PCM16 buffers")
        let started = Date()
        diagnosticsTask = Task { [weak self] in
            while !Task.isCancelled {
                do { try await Task.sleep(for: .seconds(1)) } catch { return }
                guard let self, self.epoch == generation, !self.stopping else { return }
                let now = Date()
                let stalled = now.timeIntervalSince(self.lastCapture ?? started) >= 5
                if stalled { self.inputLevel = 0 }
                self.audioWarning = stalled ? "Nenhum áudio convertido recebido há 5 s. Confira o dispositivo de entrada."
                    : (now.timeIntervalSince(self.lastSignal ?? started) >= 10 ? "Áudio chegando, mas nível muito baixo há 10 s. Confira microfone, volume e modo de voz do sistema." : nil)
                self.audioLog.info("PCM buffers=\(self.capturedBuffers) bytes=\(self.capturedBytes) sentFrames=\(self.sentFrames) rms=\(self.inputLevel) stalled=\(stalled)")
            }
        }
        inputTask = Task { [weak self] in
            for await data in stream {
                guard let self, !Task.isCancelled, self.epoch == generation, !self.stopping else { return }
                self.capturedBuffers += 1; self.capturedBytes += data.count
                self.inputLevel = VoiceDiagnostics.level(data); self.lastCapture = Date()
                if self.inputLevel > 0.001 { self.lastSignal = Date() }
                do { try await self.sendPCM(data) } catch {
                    guard !Task.isCancelled, self.epoch == generation else { return }
                    self.fail((error as? VoiceFailure) ?? .audioFormat); return
                }
            }
        }
    }
    private func sendPCM(_ data: Data) async throws {
        guard let session = conversation.sessionId else { throw VoiceFailure.disconnected }
        for frame in try frames.append(data) {
            sequence += 1
            try await socket.sendAudio(VoiceEvent(type: "audio.append", sessionId: session, turnId: inputTurn,
                                       sequence: sequence, audio: frame.base64EncodedString(), sampleRate: 16000))
        }
    }
}

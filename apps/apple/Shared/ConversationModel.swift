import SwiftUI
import AVFoundation
import VoiceCore

@MainActor final class ConversationModel: ObservableObject {
    @Published var conversation = ConversationState()
    @Published var url = Bundle.main.object(forInfoDictionaryKey: "STSGatewayURL") as? String ?? "ws://127.0.0.1:8080/v1/voice"
    @Published var token = Bundle.main.object(forInfoDictionaryKey: "STSGatewayToken") as? String ?? ""
    @Published var provider = Bundle.main.object(forInfoDictionaryKey: "STSProvider") as? String ?? "nova"
    @Published private(set) var active = false
    @Published private(set) var stopping = false
    @Published private(set) var playing = false
    @Published private(set) var errorMessage: String?
    @Published private(set) var requestId: String?
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
            try sendPCM(Data(repeating: 0, count: 16000))
            try socket.send(VoiceEvent(type: "turn.commit", sessionId: conversation.sessionId, turnId: inputTurn))
            return
        }
        let stream = try audio.start(microphone: true) { [weak self] error in
            guard self?.epoch == generation else { return }; self?.fail(error)
        }
        inputTask = Task { [weak self] in
            for await data in stream {
                guard let self, !Task.isCancelled, self.epoch == generation, !self.stopping else { return }
                do { try self.sendPCM(data) } catch { self.fail((error as? VoiceFailure) ?? .audioFormat); return }
            }
        }
    }
    private func sendPCM(_ data: Data) throws {
        guard let session = conversation.sessionId else { throw VoiceFailure.disconnected }
        for frame in try frames.append(data) {
            sequence += 1
            try socket.send(VoiceEvent(type: "audio.append", sessionId: session, turnId: inputTurn,
                                       sequence: sequence, audio: frame.base64EncodedString(), sampleRate: 16000))
        }
    }
}

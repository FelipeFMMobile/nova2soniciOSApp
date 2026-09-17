import Foundation
import OSLog

/// One ordered writer. Audio waits for capacity; persistent congestion terminates without dropping PCM.
@VoiceNetworkActor public final class VoiceSocket {
    private var onEvent: (@MainActor @Sendable (VoiceEvent) -> Void)?
    private var onFailure: (@MainActor @Sendable (VoiceFailure) -> Void)?
    private let log = Logger(subsystem: "com.sts.NovaVoice", category: "Gateway")
    private var sentAudio = 0
    private var receivedAudio = 0
    private var socket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var outbound: VoiceOutbox?
    private var reader: Task<Void, Never>?
    private var writer: Task<Void, Never>?
    private var deadline: Task<Void, Never>?
    private var generation = 0

    public nonisolated init() {}
    public func configure(onEvent: @escaping @MainActor @Sendable (VoiceEvent) -> Void,
                          onFailure: @escaping @MainActor @Sendable (VoiceFailure) -> Void) {
        self.onEvent = onEvent; self.onFailure = onFailure
    }
    public var audioFramesSent: Int { sentAudio }
    public func connect(url: String, token: String, provider: String, requestId: String) throws {
        try Task.checkCancellation()
        let endpoint = try GatewayConfiguration.validate(url)
        guard ["nova", "fake"].contains(provider), !token.utf8.contains(10), !token.utf8.contains(13), token.utf8.count <= 4096 else { throw VoiceFailure.invalidConfiguration }
        disconnect()
        sentAudio = 0; receivedAudio = 0
        log.info("Connecting voice WebSocket (credentials and endpoint omitted)")
        let epoch = generation
        let configuration = URLSessionConfiguration.ephemeral
        configuration.httpCookieStorage = nil; configuration.urlCache = nil
        configuration.timeoutIntervalForRequest = 30; configuration.timeoutIntervalForResource = 600
        let session = URLSession(configuration: configuration)
        var request = URLRequest(url: endpoint)
        if !token.isEmpty { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        let socket = session.webSocketTask(with: request)
        socket.maximumMessageSize = 512 * 1024
        self.session = session; self.socket = socket
        let outbox = VoiceOutbox()
        outbound = outbox
        writer = Task { [weak self] in
            do {
                for await event in outbox.stream {
                    try Task.checkCancellation()
                    guard self?.generation == epoch else { return }
                    let data = try event.encoded()
                    guard let text = String(data: data, encoding: .utf8) else { throw VoiceFailure.invalidEvent }
                    let started = ContinuousClock.now
                    try await socket.send(.string(text))
                    let duration = started.duration(to: .now)
                    outbox.sent()
                    guard let self, self.generation == epoch else { return }
                    if duration > .milliseconds(100) {
                        self.log.info("SEND slow duration=\(String(describing: duration), privacy: .public) queue=\(outbox.occupancy)")
                    }
                    if event.type == "audio.append" { self.sentAudio += 1 }
                    if event.type != "audio.append" || self.sentAudio == 1 || self.sentAudio % 50 == 0 {
                        self.log.info("SEND \(VoiceDiagnostics.summary(event), privacy: .public)")
                    }
                }
            } catch {
                self?.logTransportError(error, operation: "send", epoch: epoch)
                self?.fail(.disconnected, epoch: epoch)
            }
        }
        reader = Task { [weak self] in
            do {
                while !Task.isCancelled {
                    let message = try await socket.receive()
                    guard let self, self.generation == epoch else { return }
                    guard case .string(let text) = message, let data = text.data(using: .utf8) else { throw VoiceFailure.invalidEvent }
                    let event = try VoiceEvent.decode(data)
                    if event.audio != nil { self.receivedAudio += 1 }
                    if event.audio == nil || self.receivedAudio == 1 || self.receivedAudio % 50 == 0 {
                        self.log.info("RECV \(VoiceDiagnostics.summary(event), privacy: .public)")
                    }
                    if event.type == "session.ready" { self.deadline?.cancel(); self.deadline = nil }
                    await self.onEvent?(event)
                }
            } catch {
                self?.logTransportError(error, operation: "receive", epoch: epoch)
                self?.fail((error as? VoiceFailure) ?? .disconnected, epoch: epoch)
            }
        }
        deadline = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(30)); self?.fail(.disconnected, epoch: epoch) } catch {}
        }
        socket.resume()
        try send(VoiceEvent(type: "session.start", provider: provider, requestId: requestId))
    }

    public func send(_ event: VoiceEvent) throws {
        guard let outbound else { throw VoiceFailure.disconnected }
        do { try outbound.enqueueControl(event) }
        catch {
            if error as? VoiceFailure == .sendBackpressure { fail(.sendBackpressure, epoch: generation) }
            throw error
        }
    }

    /// Use sequentially from the microphone consumer; never call on the render thread.
    public func sendAudio(_ event: VoiceEvent) async throws {
        guard event.type == "audio.append" else { throw VoiceFailure.invalidEvent }
        guard let outbound else { throw VoiceFailure.disconnected }
        let epoch = generation
        let waiting = outbound.audioFull
        if waiting { log.info("SEND waiting for queue capacity (timeout=1000ms)") }
        do {
            try await outbound.enqueueAudio(event)
            if waiting { log.info("SEND queue capacity recovered") }
        } catch {
            if error as? VoiceFailure == .sendBackpressure {
                log.error("SEND queue remained full for 1000ms")
                fail(.sendBackpressure, epoch: epoch)
            }
            throw error
        }
    }

    public func disconnect() {
        if socket != nil { log.info("Disconnecting voice WebSocket") }
        generation += 1
        outbound?.close(); outbound = nil
        deadline?.cancel(); deadline = nil
        reader?.cancel(); reader = nil; writer?.cancel(); writer = nil
        socket?.cancel(with: .goingAway, reason: nil); socket = nil
        session?.invalidateAndCancel(); session = nil
    }
    private func fail(_ error: VoiceFailure, epoch: Int) {
        guard generation == epoch else { return }
        log.error("Voice WebSocket failed: \(String(describing: error), privacy: .public)")
        let callback = onFailure
        disconnect()
        Task { @MainActor in callback?(error) }
    }
    private func logTransportError(_ error: Error, operation: String, epoch: Int) {
        guard generation == epoch else { return }
        let native = error as NSError
        log.error("WebSocket \(operation, privacy: .public) failed: domain=\(native.domain, privacy: .public) code=\(native.code)")
    }
}

import Foundation

/// One ordered writer for control and audio. Overflow terminates instead of losing PCM frames.
@MainActor public final class VoiceSocket {
    public var onEvent: (@MainActor (VoiceEvent) -> Void)?
    public var onFailure: (@MainActor (VoiceFailure) -> Void)?
    private var socket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var outbound: AsyncStream<VoiceEvent>.Continuation?
    private var reader: Task<Void, Never>?
    private var writer: Task<Void, Never>?
    private var deadline: Task<Void, Never>?
    private var generation = 0

    public init() {}
    public func connect(url: String, token: String, provider: String, requestId: String) throws {
        let endpoint = try GatewayConfiguration.validate(url)
        guard ["nova", "fake"].contains(provider), !token.utf8.contains(10), !token.utf8.contains(13), token.utf8.count <= 4096 else { throw VoiceFailure.invalidConfiguration }
        disconnect()
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
        let channel = AsyncStream<VoiceEvent>.makeStream(bufferingPolicy: .bufferingOldest(16))
        outbound = channel.continuation
        writer = Task { [weak self] in
            do {
                for await event in channel.stream {
                    try Task.checkCancellation()
                    guard self?.generation == epoch else { return }
                    let data = try event.encoded()
                    guard let text = String(data: data, encoding: .utf8) else { throw VoiceFailure.invalidEvent }
                    try await socket.send(.string(text))
                }
            } catch { self?.fail(.disconnected, epoch: epoch) }
        }
        reader = Task { [weak self] in
            do {
                while !Task.isCancelled {
                    let message = try await socket.receive()
                    guard let self, self.generation == epoch else { return }
                    guard case .string(let text) = message, let data = text.data(using: .utf8) else { throw VoiceFailure.invalidEvent }
                    let event = try VoiceEvent.decode(data)
                    if event.type == "session.ready" { self.deadline?.cancel(); self.deadline = nil }
                    self.onEvent?(event)
                }
            } catch { self?.fail((error as? VoiceFailure) ?? .disconnected, epoch: epoch) }
        }
        deadline = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(30)); self?.fail(.disconnected, epoch: epoch) } catch {}
        }
        socket.resume()
        try send(VoiceEvent(type: "session.start", provider: provider, requestId: requestId))
    }

    public func send(_ event: VoiceEvent) throws {
        guard let outbound else { throw VoiceFailure.disconnected }
        switch outbound.yield(event) {
        case .enqueued: break
        case .dropped: fail(.backpressure, epoch: generation); throw VoiceFailure.backpressure
        case .terminated: throw VoiceFailure.disconnected
        @unknown default: throw VoiceFailure.disconnected
        }
    }

    public func disconnect() {
        generation += 1
        outbound?.finish(); outbound = nil
        deadline?.cancel(); deadline = nil
        reader?.cancel(); reader = nil; writer?.cancel(); writer = nil
        socket?.cancel(with: .goingAway, reason: nil); socket = nil
        session?.invalidateAndCancel(); session = nil
    }
    private func fail(_ error: VoiceFailure, epoch: Int) {
        guard generation == epoch else { return }
        disconnect(); onFailure?(error)
    }
}

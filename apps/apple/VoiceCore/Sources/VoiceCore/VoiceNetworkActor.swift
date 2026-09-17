/// Keeps packetization, JSON and WebSocket work off the UI executor.
@globalActor public actor VoiceNetworkActor {
    public static let shared = VoiceNetworkActor()
}

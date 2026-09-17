import Foundation

public struct TranscriptLine: Identifiable, Equatable, Sendable {
    public let id: String
    public let turnId: String
    public let role: String
    public let stage: String
    public var text: String
}
public struct ToolOperation: Identifiable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var status: String
    public var details: String
    public var arguments: String
    public var needsConfirmation: Bool
}
public enum VoiceEffect: Equatable, Sendable {
    case ready, clearPlayback, play(Data), renewInput, stopped
}

public struct ConversationState: Sendable {
    public private(set) var sessionId: String?
    public private(set) var outputTurnId: String?
    public private(set) var state = "disconnected"
    public private(set) var transcripts: [TranscriptLine] = []
    public private(set) var operations: [ToolOperation] = []
    public private(set) var metrics: VoiceMetrics?
    public private(set) var rejectedAudio = 0
    private var sequence: UInt64 = 0
    private var cancelled: Set<String> = []

    public init() {}
    public mutating func connecting() { state = "connecting" }
    public mutating func disconnect() { state = "disconnected"; sessionId = nil; outputTurnId = nil; sequence = 0 }

    public mutating func handle(_ event: VoiceEvent) throws -> [VoiceEffect] {
        guard event.version == 1 else { throw VoiceFailure.invalidEvent }
        if event.type == "error" { throw VoiceFailure.disconnected }
        if event.type == "session.ready" {
            guard sessionId == nil, let id = event.sessionId, Self.validId(id) else { throw VoiceFailure.invalidEvent }
            sessionId = id; state = "idle"; return [.ready]
        }
        guard let id = sessionId, event.sessionId == id else { throw VoiceFailure.sessionMismatch }
        switch event.type {
        case "session.state": state = event.state ?? state
        case "turn.started":
            guard let turn = event.turnId, Self.validId(turn), !cancelled.contains(turn) else { throw VoiceFailure.invalidEvent }
            if turn != outputTurnId { outputTurnId = turn; sequence = 0; return [.clearPlayback] }
        case "turn.interrupted":
            guard let turn = event.turnId else { throw VoiceFailure.invalidEvent }
            cancelled.insert(turn)
            if cancelled.count > 128 { throw VoiceFailure.invalidEvent }
            if turn == outputTurnId { outputTurnId = nil; sequence = 0; return [.clearPlayback] }
        case "audio.output":
            guard let turn = event.turnId, turn == outputTurnId, !cancelled.contains(turn) else { rejectedAudio += 1; return [] }
            guard event.sequence == sequence + 1 else { rejectedAudio += 1; throw VoiceFailure.audioOrder }
            guard event.sampleRate == 24000, let encoded = event.audio, encoded.utf8.count <= 350_000,
                  let data = Data(base64Encoded: encoded), !data.isEmpty, data.count % 2 == 0, data.count <= 256 * 1024 else { throw VoiceFailure.audioFormat }
            sequence += 1; return [.play(data)]
        case "transcript":
            guard let turn = event.turnId, let text = event.text, text.utf8.count <= 8192,
                  let role = event.role, ["USER", "ASSISTANT"].contains(role) else { throw VoiceFailure.invalidEvent }
            let stage = event.stage ?? "FINAL"
            let key = "\(turn)-\(role)-\(stage)"
            if role == "ASSISTANT" && stage == "FINAL" { transcripts.removeAll { $0.turnId == turn && $0.role == role && $0.stage == "SPECULATIVE" } }
            if let index = transcripts.firstIndex(where: { $0.id == key }) {
                guard transcripts[index].text.utf8.count + text.utf8.count <= 16384 else { throw VoiceFailure.invalidEvent }
                transcripts[index].text += text
            } else { transcripts.append(TranscriptLine(id: key, turnId: turn, role: role, stage: stage, text: text)) }
            if transcripts.count > 40 { transcripts.removeFirst(transcripts.count - 40) }
        case "tool.started", "tool.result", "tool.confirmation":
            guard let tool = event.tool, Self.validId(tool.operationId) else { throw VoiceFailure.invalidEvent }
            let status: String
            if event.type == "tool.started" { status = "Em execução" }
            else if event.type == "tool.confirmation" || tool.resultStatus == "confirmation_required" { status = "Aguardando confirmação" }
            else { status = tool.failed ? "Falhou ou resultado incerto" : "Concluído" }
            let pending = status == "Aguardando confirmação"
            let details = event.message ?? tool.result?.pretty ?? tool.arguments?.pretty ?? ""
            let previous = operations.first { $0.id == tool.operationId }
            let arguments = tool.arguments?.pretty ?? previous?.arguments ?? ""
            let op = ToolOperation(id: tool.operationId, name: tool.name ?? previous?.name ?? "Ferramenta MCP", status: status, details: String(details.prefix(4096)), arguments: String(arguments.prefix(4096)), needsConfirmation: pending)
            if let index = operations.firstIndex(where: { $0.id == op.id }) { operations[index] = op } else { operations.append(op) }
            // A successful matching action can use a fresh Nova tool-use ID.
            if event.type == "tool.result" && !pending && !tool.failed {
                operations.removeAll { $0.id != op.id && $0.name == op.name && $0.needsConfirmation }
            }
            if operations.count > 20 { operations.removeFirst(operations.count - 20) }
        case "turn.completed": metrics = event.metrics; state = "idle"
        case "session.renewed": outputTurnId = nil; sequence = 0; state = "idle"; return [.clearPlayback, .renewInput]
        case "session.stopped": disconnect(); return [.clearPlayback, .stopped]
        default: break // Allow future server-only informational events.
        }
        return []
    }

    public static func validId(_ id: String) -> Bool {
        !id.isEmpty && id.utf8.count <= 128 && id.utf8.allSatisfy { (65...90).contains($0) || (97...122).contains($0) || (48...57).contains($0) || $0 == 95 || $0 == 45 }
    }
}

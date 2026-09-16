import Foundation

public enum JSONValue: Codable, Equatable, Sendable {
    case object([String: JSONValue]), array([JSONValue]), string(String), number(Double), bool(Bool), null

    public init(from decoder: any Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() { self = .null }
        else if let value = try? container.decode(Bool.self) { self = .bool(value) }
        else if let value = try? container.decode(String.self) { self = .string(value) }
        else if let value = try? container.decode(Double.self) { self = .number(value) }
        else if let value = try? container.decode([String: JSONValue].self) { self = .object(value) }
        else { self = .array(try container.decode([JSONValue].self)) }
    }
    public func encode(to encoder: any Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .object(let value): try container.encode(value)
        case .array(let value): try container.encode(value)
        case .string(let value): try container.encode(value)
        case .number(let value): try container.encode(value)
        case .bool(let value): try container.encode(value)
        case .null: try container.encodeNil()
        }
    }
    public subscript(key: String) -> JSONValue? {
        guard case .object(let object) = self else { return nil }; return object[key]
    }
    public var string: String? { if case .string(let value) = self { return value }; return nil }
    public var bool: Bool? { if case .bool(let value) = self { return value }; return nil }
    public var array: [JSONValue]? { if case .array(let value) = self { return value }; return nil }
    public var pretty: String {
        let encoder = JSONEncoder(); encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return (try? encoder.encode(self)).flatMap { String(data: $0, encoding: .utf8) } ?? ""
    }
}

public struct VoiceTool: Codable, Equatable, Sendable {
    public var operationId: String
    public var name: String?
    public var arguments: JSONValue?
    public var result: JSONValue?
    public var approved: Bool?

    public init(operationId: String, name: String? = nil, arguments: JSONValue? = nil, result: JSONValue? = nil, approved: Bool? = nil) {
        self.operationId = operationId; self.name = name; self.arguments = arguments; self.result = result; self.approved = approved
    }
    public var resultStatus: String? {
        guard let text = result?["content"]?.array?.first?["text"]?.string,
              let data = text.data(using: .utf8), let value = try? JSONDecoder().decode(JSONValue.self, from: data) else { return nil }
        return value["status"]?.string
    }
    public var failed: Bool { result?["isError"]?.bool == true || resultStatus == "error" }
}

public struct VoiceMetrics: Codable, Equatable, Sendable {
    public var firstAudioMs: Double?
    public var durationMs: Double?
    public var interruptionMs: Double?
}

public struct VoiceEvent: Codable, Equatable, Sendable {
    public var version: Int = 1
    public var type: String
    public var sessionId: String?
    public var turnId: String?
    public var sequence: UInt64?
    public var provider: String?
    public var requestId: String?
    public var state: String?
    public var audio: String?
    public var sampleRate: Int?
    public var text: String?
    public var role: String?
    public var stage: String?
    public var code: String?
    public var message: String?
    public var metrics: VoiceMetrics?
    public var tool: VoiceTool?

    public init(type: String, sessionId: String? = nil, turnId: String? = nil, sequence: UInt64? = nil,
                provider: String? = nil, requestId: String? = nil, state: String? = nil, audio: String? = nil,
                sampleRate: Int? = nil, text: String? = nil, role: String? = nil, stage: String? = nil,
                tool: VoiceTool? = nil) {
        self.type = type; self.sessionId = sessionId; self.turnId = turnId; self.sequence = sequence
        self.provider = provider; self.requestId = requestId; self.state = state; self.audio = audio
        self.sampleRate = sampleRate; self.text = text; self.role = role; self.stage = stage; self.tool = tool
    }

    public static func decode(_ data: Data) throws -> Self {
        guard data.count <= 512 * 1024 else { throw VoiceFailure.invalidEvent }
        let event = try JSONDecoder().decode(Self.self, from: data)
        guard event.version == 1 else { throw VoiceFailure.invalidEvent }; return event
    }
    public func encoded() throws -> Data { try JSONEncoder().encode(self) }
}

public enum VoiceFailure: Error, LocalizedError, Equatable, Sendable {
    case invalidEvent, invalidConfiguration, sessionMismatch, audioFormat, audioOrder, backpressure, disconnected, microphoneDenied
    public var errorDescription: String? {
        switch self {
        case .invalidEvent: return "O gateway enviou um evento inválido ou incompatível."
        case .invalidConfiguration: return "Informe uma URL ws:// ou wss:// com caminho /v1/voice, sem credenciais ou parâmetros."
        case .sessionMismatch: return "O evento pertence a outra sessão. Reconecte."
        case .audioFormat: return "O áudio recebido não é PCM16 mono a 24 kHz."
        case .audioOrder: return "A sequência de áudio está incompleta. Reinicie a conversa."
        case .backpressure: return "A fila de áudio/rede excedeu o limite. A conversa foi encerrada."
        case .disconnected: return "A conexão foi perdida. Confira o gateway e o token local."
        case .microphoneDenied: return "Permita o uso do microfone nos Ajustes do sistema."
        }
    }
}

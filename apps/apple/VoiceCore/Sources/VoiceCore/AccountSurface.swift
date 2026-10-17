import Foundation

/// Restricted A2UI v0.9.1 catalog. It never evaluates URLs, scripts or bindings.
public struct AccountSurface: Equatable, Sendable {
    public static let welcome = "Sou agente STS para logar sua conta, deseja continuar?"
    public private(set) var visible = false
    public private(set) var instructions = ""
    public private(set) var transactionId: String?
    public init() {}
    private func exact(_ value: JSONValue, _ keys: Set<String>) throws {
        guard case .object(let object) = value, Set(object.keys) == keys else { throw VoiceFailure.invalidEvent }
    }
    public mutating func apply(_ value: JSONValue) throws {
        let messages = value.array ?? [value]
        guard messages.count <= 8 else { throw VoiceFailure.invalidEvent }
        var next = self
        for message in messages {
            guard message["version"]?.string == "v0.9.1" else { throw VoiceFailure.invalidEvent }
            if let create = message["createSurface"] {
                try exact(message, ["version", "createSurface"]); try exact(create, ["surfaceId", "catalogId"])
                guard !next.visible, create["surfaceId"]?.string == "account-login", create["catalogId"]?.string == "urn:sts:a2ui:account:v1" else { throw VoiceFailure.invalidEvent }
                next = AccountSurface(); next.visible = true
            } else if let update = message["updateComponents"] {
                try exact(message, ["version", "updateComponents"]); try exact(update, ["surfaceId", "components"])
                guard next.visible, update["surfaceId"]?.string == "account-login", let components = update["components"]?.array, components.count == 4 else { throw VoiceFailure.invalidEvent }
                var seen = Set<String>()
                for component in components {
                    guard let id = component["id"]?.string, seen.insert(id).inserted else { throw VoiceFailure.invalidEvent }
                    switch (id, component["component"]?.string) {
                    case ("root", "Column"):
                        try exact(component, ["id", "component", "children"])
                        guard let children = component["children"]?.array, children.count == 2,
                              children.compactMap(\.string) == ["instructions", "login"] else { throw VoiceFailure.invalidEvent }
                    case ("instructions", "Text"):
                        try exact(component, ["id", "component", "text"])
                        guard let text = component["text"]?.string, text.count <= 500 else { throw VoiceFailure.invalidEvent }; next.instructions = text
                    case ("label", "Text"):
                        try exact(component, ["id", "component", "text"])
                        guard component["text"]?.string == "Autenticar" else { throw VoiceFailure.invalidEvent }
                    case ("login", "Button"):
                        try exact(component, ["id", "component", "child", "action"])
                        guard let action = component["action"], let event = action["event"], let context = event["context"] else { throw VoiceFailure.invalidEvent }
                        try exact(action, ["event"]); try exact(event, ["name", "context"]); try exact(context, ["transaction_id"])
                        guard component["child"]?.string == "label", event["name"]?.string == "account.authenticate", let transaction = event["context"]?["transaction_id"]?.string, transaction.count == 43, transaction.allSatisfy({ $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "-" || $0 == "_") }) else { throw VoiceFailure.invalidEvent }
                        next.transactionId = transaction
                    default: throw VoiceFailure.invalidEvent
                    }
                }
                guard seen == ["root", "instructions", "label", "login"] else { throw VoiceFailure.invalidEvent }
            } else if let delete = message["deleteSurface"] {
                try exact(message, ["version", "deleteSurface"]); try exact(delete, ["surfaceId"])
                guard delete["surfaceId"]?.string == "account-login" else { throw VoiceFailure.invalidEvent }; next = AccountSurface()
            } else { throw VoiceFailure.invalidEvent }
        }
        self = next
    }
}

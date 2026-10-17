import XCTest
@testable import VoiceCore
final class AccountSurfaceTests: XCTestCase {
    func value(_ text: String) throws -> JSONValue { try JSONDecoder().decode(JSONValue.self, from: Data(text.utf8)) }
    func testExplicitVersionNegotiation() throws {
        let data = Data(#"{"version":2,"type":"session.ready"}"#.utf8)
        XCTAssertThrowsError(try VoiceEvent.decode(data))
        XCTAssertEqual(try VoiceEvent.decode(data, expectedVersion: 2).version, 2)
    }
    func testSurfaceLifecycleRejectsUnknownCatalogAndActions() throws {
        var surface = AccountSurface()
        XCTAssertThrowsError(try surface.apply(value(#"{"version":"v0.9.1","createSurface":{"surfaceId":"account-login","catalogId":"untrusted"}}"#)))
        try surface.apply(value(#"{"version":"v0.9.1","createSurface":{"surfaceId":"account-login","catalogId":"urn:sts:a2ui:account:v1"}}"#))
        XCTAssertTrue(surface.visible)
        XCTAssertThrowsError(try surface.apply(value(#"{"version":"v0.9.1","updateComponents":{"surfaceId":"account-login","components":[{"id":"root","component":"WebView","url":"https://evil.example"}]}}"#)))
        XCTAssertThrowsError(try surface.apply(value(#"{"version":"v0.9.1","deleteSurface":{"surfaceId":"account-login"},"url":"https://evil.example"}"#)))
        XCTAssertTrue(surface.visible)
        try surface.apply(value(#"{"version":"v0.9.1","deleteSurface":{"surfaceId":"account-login"}}"#))
        XCTAssertFalse(surface.visible); XCTAssertNil(surface.transactionId)
    }

    func testValidLoginCardAndRejectedUpdateAreAtomic() throws {
        let transaction = String(repeating: "a", count: 43)
        let card = """
        [{"version":"v0.9.1","deleteSurface":{"surfaceId":"account-login"}},
         {"version":"v0.9.1","createSurface":{"surfaceId":"account-login","catalogId":"urn:sts:a2ui:account:v1"}},
         {"version":"v0.9.1","updateComponents":{"surfaceId":"account-login","components":[
          {"id":"root","component":"Column","children":["instructions","login"]},
          {"id":"instructions","component":"Text","text":"Entre pela tela segura"},
          {"id":"label","component":"Text","text":"Autenticar"},
          {"id":"login","component":"Button","child":"label","action":{"event":{"name":"account.authenticate","context":{"transaction_id":"\(transaction)"}}}}
         ]}}]
        """
        var surface = AccountSurface()
        try surface.apply(value(card))
        XCTAssertTrue(surface.visible)
        XCTAssertEqual(surface.instructions, "Entre pela tela segura")
        XCTAssertEqual(surface.transactionId, transaction)
        let original = surface
        for malformed in [
            card.replacingOccurrences(of: #"["instructions","login"]"#, with: #"["instructions","login",null]"#),
            card.replacingOccurrences(of: "account.authenticate", with: "account.open_url"),
            card.replacingOccurrences(of: transaction, with: "invalid")
        ] {
            XCTAssertThrowsError(try surface.apply(value(malformed)))
            XCTAssertEqual(surface, original)
        }
        // A replacement login request clears the old transaction before creation.
        let renewed = String(repeating: "b", count: 43)
        try surface.apply(value(card.replacingOccurrences(of: transaction, with: renewed)))
        XCTAssertEqual(surface.transactionId, renewed)
    }
}

import XCTest

@MainActor final class NovaVoiceUITests: XCTestCase {
    func testFakeConversationAndStop() {
        let app = XCUIApplication()
        app.launchArguments = ["--ui-testing"]
        app.launch()
        let button = app.buttons["conversation.toggle"]
        XCTAssertTrue(button.waitForExistence(timeout: 10))
        XCTAssertEqual(button.label, "Iniciar conversa")
        button.tap()
        XCTAssertTrue(app.staticTexts["Olá! Esta é uma resposta simulada. O gateway está funcionando."].waitForExistence(timeout: 15))
        XCTAssertEqual(button.label, "Encerrar conversa")
        button.tap()
        let idle = NSPredicate(format: "label == %@", "Iniciar conversa")
        expectation(for: idle, evaluatedWith: button)
        waitForExpectations(timeout: 5)
        XCTAssertFalse(app.otherElements["conversation.error"].exists)
    }
}

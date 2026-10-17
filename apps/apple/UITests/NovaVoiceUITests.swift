import XCTest

@MainActor final class NovaVoiceUITests: XCTestCase {
    func testFakeConversationAndStop() {
        let app = XCUIApplication()
        app.launchArguments = ["--ui-testing"]
        for key in ["STS_TEST_URL", "STS_TEST_TOKEN"] { app.launchEnvironment[key] = ProcessInfo.processInfo.environment[key] }
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

    func testAccountLoginRestoreLogoutAndAgentSwitch() throws {
        try XCTSkipUnless(ProcessInfo.processInfo.environment["STS_ACCOUNT_UI"] == "1", "Requires isolated account smoke stack")
        let app = XCUIApplication()
        app.launchArguments = ["--ui-testing"]
        for key in ["STS_TEST_URL", "STS_TEST_TOKEN"] { app.launchEnvironment[key] = ProcessInfo.processInfo.environment[key] }
        app.launch()
        let agents = app.segmentedControls["conversation.agent"]
        XCTAssertTrue(agents.waitForExistence(timeout: 10))
        agents.buttons["Conta"].tap()
        let toggle = app.buttons["conversation.toggle"]
        toggle.tap()
        // A prior interrupted test may have left this installation signed in.
        if app.buttons["account.logout"].waitForExistence(timeout: 2) {
            app.buttons["account.logout"].tap()
            let idle = NSPredicate(format: "label == %@", "Iniciar conversa")
            expectation(for: idle, evaluatedWith: toggle)
            waitForExpectations(timeout: 5)
            toggle.tap()
        }
        let proceed = app.buttons["account.continue"]
        XCTAssertTrue(proceed.waitForExistence(timeout: 15))
        XCTAssertFalse(app.buttons["account.authenticate"].exists)
        app.buttons["account.decline"].tap()
        let declined = NSPredicate(format: "label == %@", "Iniciar conversa")
        expectation(for: declined, evaluatedWith: toggle)
        waitForExpectations(timeout: 5)
        XCTAssertFalse(app.buttons["account.authenticate"].exists)
        toggle.tap()
        XCTAssertTrue(proceed.waitForExistence(timeout: 15))
        proceed.tap()
        XCTAssertTrue(app.staticTexts["Entre com seus dados"].waitForExistence(timeout: 15))
        let authenticate = app.buttons["account.authenticate"]
        guard authenticate.waitForExistence(timeout: 15) else {
            XCTFail("Login card missing: \(app.debugDescription)")
            return
        }
        XCTAssertTrue(authenticate.isEnabled)
        authenticate.tap()
        XCTAssertTrue(app.buttons["account.logout"].waitForExistence(timeout: 15))
        XCTAssertTrue(app.staticTexts["Conta autenticada"].exists)
        XCTAssertTrue(app.staticTexts["Conta conectada com sucesso, agora você pode consultar e editar informações da sua conta"].waitForExistence(timeout: 15))
        XCTAssertFalse(authenticate.exists)
        XCTAssertFalse(app.otherElements["conversation.error"].exists)

        // Process restart must restore from Keychain through refresh, without login UI.
        app.terminate()
        app.launch()
        app.segmentedControls["conversation.agent"].buttons["Conta"].tap()
        app.buttons["conversation.toggle"].tap()
        XCTAssertTrue(app.buttons["account.logout"].waitForExistence(timeout: 15))
        XCTAssertTrue(app.staticTexts["Conta conectada com sucesso, agora você pode consultar e editar informações da sua conta"].waitForExistence(timeout: 15))
        XCTAssertFalse(app.buttons["account.authenticate"].exists)
        app.buttons["account.logout"].tap()
        let signedOut = NSPredicate(format: "exists == false")
        expectation(for: signedOut, evaluatedWith: app.buttons["account.logout"])
        waitForExpectations(timeout: 10)

        // Changing agent stops the old session and keeps Agenda/notas usable.
        app.segmentedControls["conversation.agent"].buttons["Agenda / notas"].tap()
        app.buttons["conversation.toggle"].tap()
        XCTAssertTrue(app.staticTexts["Olá! Esta é uma resposta simulada. O gateway está funcionando."].waitForExistence(timeout: 15))
        app.segmentedControls["conversation.agent"].buttons["Conta"].tap()
        XCTAssertEqual(app.buttons["conversation.toggle"].label, "Iniciar conversa")
        app.buttons["conversation.toggle"].tap()
        XCTAssertTrue(app.buttons["account.continue"].waitForExistence(timeout: 15))
        app.buttons["account.continue"].tap()
        XCTAssertTrue(app.buttons["account.authenticate"].waitForExistence(timeout: 15))
        XCTAssertFalse(app.buttons["account.logout"].exists)
        XCTAssertFalse(app.otherElements["conversation.error"].exists)
    }
}

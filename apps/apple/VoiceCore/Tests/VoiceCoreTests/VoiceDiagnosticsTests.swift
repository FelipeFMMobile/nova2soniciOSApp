import XCTest
@testable import VoiceCore

final class VoiceDiagnosticsTests: XCTestCase {
    func testPCMLevel() {
        XCTAssertEqual(VoiceDiagnostics.level(Data(repeating: 0, count: 640)), 0)
        XCTAssertEqual(VoiceDiagnostics.level(Data([0, 64, 0, 192])), 0.5, accuracy: 0.0001)
        XCTAssertEqual(VoiceDiagnostics.level(Data()), 0)
    }
    func testSummaryDoesNotIncludeContent() {
        let event = VoiceEvent(type: "audio.append", sequence: 42, audio: "SECRET_AUDIO", text: "SECRET_TEXT",
                               tool: VoiceTool(operationId: "SECRET_ID", arguments: .string("SECRET_ARGS")))
        let summary = VoiceDiagnostics.summary(event)
        XCTAssertTrue(summary.contains("sequence=42"))
        XCTAssertFalse(summary.contains("SECRET"))
    }
}

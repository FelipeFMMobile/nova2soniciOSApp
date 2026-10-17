import Foundation
import XCTest
@testable import VoiceCore

final class WebAuthenticationCallbackTests: XCTestCase {
    @MainActor func testBackgroundCallbackResumesLoginOnMainActor() async throws {
        let expected = URL(string: "stsvoice://oauth/callback?code=synthetic&state=synthetic")!
        let result: URL = try await withCheckedThrowingContinuation { continuation in
            let completion = WebAuthenticationCallback.completion(for: continuation)
            DispatchQueue.global().async {
                dispatchPrecondition(condition: .notOnQueue(.main))
                completion(expected, nil)
            }
        }
        MainActor.assertIsolated()
        XCTAssertEqual(result, expected)
    }

    @MainActor func testBackgroundCancellationReturnsFailureOnMainActor() async {
        do {
            let _: URL = try await withCheckedThrowingContinuation { continuation in
                let completion = WebAuthenticationCallback.completion(for: continuation)
                DispatchQueue.global().async {
                    dispatchPrecondition(condition: .notOnQueue(.main))
                    completion(nil, NSError(domain: "test.cancelled", code: 1))
                }
            }
            XCTFail("Cancelled authentication must fail")
        } catch {
            MainActor.assertIsolated()
            XCTAssertEqual(error as? VoiceFailure, .disconnected)
        }
    }
}

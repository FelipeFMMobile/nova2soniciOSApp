import Foundation
import XCTest
@testable import VoiceCore

final class AccountBrowserPolicyTests: XCTestCase {
    func testOnlyConfiguredHTTPSOriginCanNavigate() {
        let origin = URL(string: "https://pool.auth.us-east-1.amazoncognito.com/oauth2/authorize")!
        for path in ["/login", "/mfa", "/newPassword?x=1"] {
            XCTAssertEqual(AccountBrowserPolicy.navigation(URL(string: path, relativeTo: origin)!.absoluteURL, origin: origin), .allow)
        }
        for raw in ["http://pool.auth.us-east-1.amazoncognito.com/login", "https://evil.example/login", "https://pool.auth.us-east-1.amazoncognito.com.evil.example/", "https://pool.auth.us-east-1.amazoncognito.com:444/login", "https://user:pass@pool.auth.us-east-1.amazoncognito.com/", "javascript:alert(1)", "file:///tmp/test", "stsvoice://wrong/callback", "stsvoice://oauth/other"] {
            XCTAssertEqual(AccountBrowserPolicy.navigation(URL(string: raw)!, origin: origin), .block, raw)
        }
        XCTAssertEqual(AccountBrowserPolicy.navigation(URL(string: "stsvoice://oauth/callback?code=synthetic&state=synthetic")!, origin: origin), .callback)
    }
}

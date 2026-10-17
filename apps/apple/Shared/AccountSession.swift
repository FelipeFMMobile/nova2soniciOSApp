import Foundation
import WebKit
import Security
import SwiftUI
import VoiceCore

@MainActor final class AccountSession: NSObject, ObservableObject {
    @Published private(set) var authenticated = false
    @Published private(set) var busy = false
    @Published private(set) var error: String?
    private(set) var access: String?
    private var expiresAt: TimeInterval = 0
    @Published private(set) var browserURL: URL?
    private var browserContinuation: CheckedContinuation<URL, any Error>?
    private var loginAttempt: UUID?
    private var gateway = ""
    private var gatewayToken = ""
    private var storageScope = ""
    private var deviceValue = ""

    var device: String { deviceValue }
    private func key(_ name: String) -> String { storageScope + ":" + name }
    private func query(_ name: String) -> [String: Any] { [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "com.sts.account", kSecAttrAccount as String: key(name), kSecAttrSynchronizable as String: false] }
    private func load(_ name: String) -> String? {
        var q = query(name); q[kSecReturnData as String] = true; q[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?; guard SecItemCopyMatching(q as CFDictionary, &result) == errSecSuccess, let data = result as? Data else { return nil }; return String(data: data, encoding: .utf8)
    }
    private func save(_ name: String, _ value: String?) throws {
        let q = query(name)
        guard let value else { SecItemDelete(q as CFDictionary); return }
        let data = Data(value.utf8)
        let updated = SecItemUpdate(q as CFDictionary, [kSecValueData as String: data] as CFDictionary)
        if updated == errSecItemNotFound { var add = q; add[kSecValueData as String] = data; add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly; guard SecItemAdd(add as CFDictionary, nil) == errSecSuccess else { throw VoiceFailure.invalidConfiguration } }
        else if updated != errSecSuccess { throw VoiceFailure.invalidConfiguration }
    }
    func configure(url: String, token: String) throws {
        guard var u = URLComponents(string: url), let host = u.host, ["ws", "wss"].contains(u.scheme ?? "") else { throw VoiceFailure.invalidConfiguration }
        guard u.scheme == "wss" || ["localhost", "127.0.0.1", "::1"].contains(host) else { throw VoiceFailure.invalidConfiguration }
        u.scheme = u.scheme == "wss" ? "https" : "http"; u.path = ""; u.query = nil; u.fragment = nil
        guard let base = u.string else { throw VoiceFailure.invalidConfiguration }
        if gateway != base { access = nil; authenticated = false; expiresAt = 0 }
        gateway = base; gatewayToken = token; storageScope = base
        if let stored = load("device") { deviceValue = stored }
        else { var bytes = [UInt8](repeating: 0, count: 32); guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else { throw VoiceFailure.invalidConfiguration }; deviceValue = Data(bytes).base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: ""); try save("device", deviceValue) }
    }
    private func post(_ action: String, _ body: [String: String]) async throws -> [String: Any] {
        guard let url = URL(string: gateway + "/v2/auth/" + action) else { throw VoiceFailure.invalidConfiguration }
        var request = URLRequest(url: url); request.httpMethod = "POST"; request.timeoutInterval = 20
        request.setValue("Bearer " + gatewayToken, forHTTPHeaderField: "Authorization"); request.setValue(device, forHTTPHeaderField: "X-STS-Device"); request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        let configuration = URLSessionConfiguration.ephemeral; configuration.httpCookieStorage = nil; configuration.urlCache = nil
        let session = URLSession(configuration: configuration); defer { session.invalidateAndCancel() }
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse, response.statusCode == 200, data.count <= 32768, let result = try JSONSerialization.jsonObject(with: data) as? [String: Any], result["error"] == nil else { throw VoiceFailure.disconnected }
        return result
    }
    private func accept(_ result: [String: Any]) throws {
        guard let access = result["access_token"] as? String, let refresh = result["refresh_token"] as? String, let expiry = result["expires_at"] as? TimeInterval, expiry > Date().timeIntervalSince1970 else { throw VoiceFailure.invalidEvent }
        try save("refresh", refresh); self.access = access; expiresAt = expiry; authenticated = true
    }
    func restore() async {
        guard expiresAt < Date().timeIntervalSince1970 + 30, let refresh = load("refresh") else { return }
        do { try accept(await post("refresh", ["refresh_token": refresh])) }
        catch { self.access = nil; authenticated = false; self.error = "Não foi possível renovar a sessão. Tente novamente." }
    }
    func login(transaction: String) async -> Bool {
        guard !busy else { return false }; busy = true; error = nil
        let attempt = UUID(); loginAttempt = attempt
        defer { busy = false; browserURL = nil; loginAttempt = nil }
        do {
            let prepared = try await post("prepare", ["transaction_id": transaction])
            guard loginAttempt == attempt else { throw CancellationError() }
            guard let raw = prepared["authorization_url"] as? String, let url = URL(string: raw) else { throw VoiceFailure.invalidEvent }
            let callback: URL
            #if DEBUG
            if prepared["fake"] as? Bool == true, url.scheme == "stsvoice", URL(string: gateway)?.host == "127.0.0.1" { callback = url }
            else { callback = try await authenticate(url) }
            #else
            callback = try await authenticate(url)
            #endif
            guard callback.scheme == "stsvoice", callback.host == "oauth", callback.path == "/callback", let query = URLComponents(url: callback, resolvingAgainstBaseURL: false)?.queryItems else { throw VoiceFailure.invalidEvent }
            guard query.filter({ $0.name == "code" }).count == 1, query.filter({ $0.name == "state" }).count == 1, let code = query.first(where: { $0.name == "code" })?.value, let state = query.first(where: { $0.name == "state" })?.value else { throw VoiceFailure.invalidEvent }
            guard loginAttempt == attempt else { throw CancellationError() }
            let result = try await post("complete", ["transaction_id": transaction, "state": state, "code": code])
            guard loginAttempt == attempt else { throw CancellationError() }
            try accept(result); return true
        } catch { self.error = "Autenticação não concluída. Tente novamente pelo agente Conta."; return false }
    }
    private func authenticate(_ url: URL) async throws -> URL {
        guard url.scheme == "https", url.host != nil else { throw VoiceFailure.invalidConfiguration }
        return try await withCheckedThrowingContinuation { continuation in
            browserContinuation = continuation
            browserURL = url
        }
    }
    func finishBrowser(_ callback: URL?) {
        guard let continuation = browserContinuation else { return }
        browserContinuation = nil; browserURL = nil
        if let callback { continuation.resume(returning: callback) }
        else { continuation.resume(throwing: VoiceFailure.disconnected) }
    }
    func cancelLogin() { loginAttempt = nil; finishBrowser(nil) }
    func clearRevokedSession() { access = nil; expiresAt = 0; authenticated = false; try? save("refresh", nil) }
    func logout() async {
        if let refresh = load("refresh") {
            do { _ = try await post("logout", ["refresh_token": refresh]) }
            catch { self.error = "Não foi possível encerrar a sessão no servidor. Tente sair novamente."; return }
        }
        access = nil; expiresAt = 0; authenticated = false; try? save("refresh", nil)
    }
}

struct AccountPanel: View {
    @ObservedObject var model: ConversationModel
    @ObservedObject var account: AccountSession
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Label(account.authenticated ? "Conta autenticada" : "Entre para acessar seus dados", systemImage: account.authenticated ? "person.crop.circle.badge.checkmark" : "lock.shield")
            if model.provider == "fake" { Text("Demonstração local com usuários fictícios. Não autentica na AWS.").font(.caption) }
            if model.awaitingAccountConsent {
                Text(AccountSurface.welcome)
                HStack {
                    Button("Sim, continuar") { model.answerAccountWelcome(true) }.accessibilityIdentifier("account.continue")
                    Button("Não, encerrar") { model.answerAccountWelcome(false) }.accessibilityIdentifier("account.decline")
                }
            }
            if account.busy { ProgressView("Autenticando…") }
            if let error = account.error { Text(error).foregroundStyle(.orange) }
            if account.authenticated { Button("Sair") { model.signOutAccount() }.accessibilityIdentifier("account.logout") }
        }.padding().frame(maxWidth: .infinity, alignment: .leading).background(.white.opacity(0.05), in: RoundedRectangle(cornerRadius: 16))
    }
}


struct AccountLoginModal: View {
    @ObservedObject var model: ConversationModel
    @ObservedObject var account: AccountSession
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                Text("Entre com seus dados").font(.title2.bold())
                Spacer()
                Button("Cancelar") { model.dismissAccountLogin() }.accessibilityIdentifier("account.cancel")
            }
            if let url = account.browserURL {
                Label(url.host ?? "Cognito", systemImage: "lock.shield").font(.caption).textSelection(.enabled)
                AccountWebBrowser(url: url, account: account)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .accessibilityIdentifier("account.browser")
            } else {
                Text(model.accountSurface.instructions)
                if let error = account.error { Text(error).foregroundStyle(.orange) }
                Button("Autenticar") { model.authenticateAccount() }
                    .buttonStyle(.borderedProminent).disabled(account.busy)
                    .accessibilityIdentifier("account.authenticate")
                if account.busy { ProgressView("Autenticando…") }
                Spacer()
            }
        }.padding(24)
            .frame(minWidth: 320, idealWidth: 520, maxWidth: .infinity, minHeight: 480, idealHeight: 640, maxHeight: .infinity)
            .interactiveDismissDisabled(account.busy)
    }
}

#if os(macOS)
private typealias AccountBrowserRepresentable = NSViewRepresentable
#else
private typealias AccountBrowserRepresentable = UIViewRepresentable
#endif

private struct AccountWebBrowser: AccountBrowserRepresentable {
    let url: URL
    let account: AccountSession
    func makeCoordinator() -> Coordinator { Coordinator(url: url, account: account) }
    private func makeBrowser(_ context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        let browser = WKWebView(frame: .zero, configuration: configuration)
        browser.navigationDelegate = context.coordinator
        browser.load(URLRequest(url: url))
        return browser
    }
    #if os(macOS)
    func makeNSView(context: Context) -> WKWebView { makeBrowser(context) }
    func updateNSView(_ view: WKWebView, context: Context) {}
    static func dismantleNSView(_ view: WKWebView, coordinator: Coordinator) { view.stopLoading(); view.navigationDelegate = nil }
    #else
    func makeUIView(context: Context) -> WKWebView { makeBrowser(context) }
    func updateUIView(_ view: WKWebView, context: Context) {}
    static func dismantleUIView(_ view: WKWebView, coordinator: Coordinator) { view.stopLoading(); view.navigationDelegate = nil }
    #endif
    @MainActor final class Coordinator: NSObject, WKNavigationDelegate {
        let origin: URL
        let account: AccountSession
        init(url: URL, account: AccountSession) { origin = url; self.account = account }
        func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
                     decisionHandler: @escaping @MainActor @Sendable (WKNavigationActionPolicy) -> Void) {
            guard let target = navigationAction.request.url else { decisionHandler(.cancel); return }
            switch AccountBrowserPolicy.navigation(target, origin: origin) {
            case .callback:
                decisionHandler(.cancel)
                account.finishBrowser(target)
            case .allow: decisionHandler(.allow)
            case .block:
                decisionHandler(.cancel)
                account.finishBrowser(nil)
            }
        }
        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: any Error) {
            if (error as NSError).code != NSURLErrorCancelled { account.finishBrowser(nil) }
        }
        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: any Error) {
            if (error as NSError).code != NSURLErrorCancelled { account.finishBrowser(nil) }
        }
        func webViewWebContentProcessDidTerminate(_ webView: WKWebView) { account.finishBrowser(nil) }
    }
}

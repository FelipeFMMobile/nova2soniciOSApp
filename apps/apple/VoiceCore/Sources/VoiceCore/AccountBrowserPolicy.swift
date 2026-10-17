import Foundation

public enum AccountBrowserPolicy {
    public enum Decision: Equatable, Sendable { case allow, callback, block }
    public static func navigation(_ url: URL, origin: URL) -> Decision {
        guard url.user == nil, url.password == nil else { return .block }
        if url.scheme == "stsvoice", url.host == "oauth", url.path == "/callback", url.port == nil {
            return .callback
        }
        guard origin.scheme == "https", url.scheme == "https", let host = origin.host,
              url.host == host, (url.port ?? 443) == (origin.port ?? 443) else { return .block }
        return .allow
    }
}

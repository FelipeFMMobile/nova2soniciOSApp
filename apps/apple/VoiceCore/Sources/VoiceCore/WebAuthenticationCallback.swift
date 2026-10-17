import Foundation

/// AuthenticationServices can invoke its Objective-C completion on a background queue.
/// Keep this boundary Sendable; the awaiting task resumes on its own actor.
public enum WebAuthenticationCallback {
    public static func completion(
        for continuation: CheckedContinuation<URL, any Error>
    ) -> @Sendable (URL?, (any Error)?) -> Void {
        { callback, _ in
            if let callback {
                continuation.resume(returning: callback)
            } else {
                continuation.resume(throwing: VoiceFailure.disconnected)
            }
        }
    }
}

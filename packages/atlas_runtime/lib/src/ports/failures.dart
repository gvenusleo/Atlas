/// A failure whose message is safe to surface to users and protocol clients.
///
/// Implementations guarantee [safeMessage] is a redacted, provider-safe
/// description that never carries raw exception text, tool arguments, or
/// model output. Runtime summaries and protocol adapters use it to report
/// failures without leaking request payloads.
abstract interface class SafeMessageException implements Exception {
  /// A redacted, human-readable description of the failure.
  String get safeMessage;
}

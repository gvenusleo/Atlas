import '../domain/ids.dart';

/// Severity for structured Atlas log events.
enum LogLevel {
  /// Diagnostic details useful during development.
  debug,

  /// Normal lifecycle information.
  info,

  /// Recoverable abnormal behavior.
  warn,

  /// An operation failed.
  error,
}

/// A redacted, structured diagnostic event.
final class const LogEvent({
  /// Event severity.
  required final LogLevel level,

  /// Stable event code.
  required final String code,

  /// Redacted human-readable message.
  required final String message,

  /// Related session, when available.
  final SessionId? sessionId,

  /// Related turn, when available.
  final TurnId? turnId,

  /// Redacted structured fields.
  final Map<String, Object?> fields = const <String, Object?>{},

  /// Event timestamp.
  required final DateTime occurredAt,
}) {
  /// Creates a log event.
  this;
}

/// Logging port used by runtime and adapters.
abstract interface class AtlasLogger {
  /// Records one already-redacted event.
  void log(LogEvent event);
}

/// Default logger that intentionally discards events.
final class const NoopLogger() implements AtlasLogger {
  /// Creates a no-op logger.
  this;

  @override
  void log(LogEvent event) {}
}

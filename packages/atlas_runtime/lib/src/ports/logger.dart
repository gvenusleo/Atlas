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
final class LogEvent {
  /// Creates a log event.
  const LogEvent({
    required this.level,
    required this.code,
    required this.message,
    this.sessionId,
    this.turnId,
    this.fields = const <String, Object?>{},
    required this.occurredAt,
  });

  /// Event severity.
  final LogLevel level;

  /// Stable event code.
  final String code;

  /// Redacted human-readable message.
  final String message;

  /// Related session, when available.
  final SessionId? sessionId;

  /// Related turn, when available.
  final TurnId? turnId;

  /// Redacted structured fields.
  final Map<String, Object?> fields;

  /// Event timestamp.
  final DateTime occurredAt;
}

/// Logging port used by runtime and adapters.
abstract interface class AtlasLogger {
  /// Records one already-redacted event.
  void log(LogEvent event);
}

/// Default logger that intentionally discards events.
final class NoopLogger implements AtlasLogger {
  /// Creates a no-op logger.
  const NoopLogger();

  @override
  void log(LogEvent event) {}
}

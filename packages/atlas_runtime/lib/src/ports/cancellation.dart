/// A cooperative cancellation signal shared by a model turn and its tools.
final class CancellationToken {
  bool _cancelled = false;

  /// Whether cancellation has been requested.
  bool get isCancelled => _cancelled;

  /// Requests cancellation for the current operation.
  void cancel() => _cancelled = true;

  /// Throws when cancellation has been requested.
  void throwIfCancelled() {
    if (_cancelled) {
      throw const TurnCancelledException();
    }
  }
}

/// Raised when cooperative turn cancellation reaches an operation.
final class TurnCancelledException implements Exception {
  /// Creates a cancellation error.
  const TurnCancelledException();

  @override
  String toString() => 'Turn cancelled';
}

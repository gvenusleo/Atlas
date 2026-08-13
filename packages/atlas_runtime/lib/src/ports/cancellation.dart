import 'dart:async';

/// A cooperative cancellation signal shared by a model turn and its tools.
final class CancellationToken {
  bool _cancelled = false;
  final Completer<void> _cancelledCompleter = Completer<void>();

  /// Whether cancellation has been requested.
  bool get isCancelled => _cancelled;

  /// Completes when cancellation has been requested.
  Future<void> get whenCancelled => _cancelledCompleter.future;

  /// Requests cancellation for the current operation.
  void cancel() {
    if (_cancelled) {
      return;
    }
    _cancelled = true;
    _cancelledCompleter.complete();
  }

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

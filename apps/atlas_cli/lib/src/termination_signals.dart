import 'dart:async';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

/// Owns termination listeners for a single command invocation.
final class TerminationSignals() {
  /// Starts watching the signals supported by this platform.
  this {
    for (final signal in [
      ProcessSignal.sigint,
      if (!Platform.isWindows) ProcessSignal.sigterm,
    ]) {
      _subscriptions.add(
        signal.watch().listen((_) {
          cancellation.cancel();
          if (!_interrupted.isCompleted) _interrupted.complete();
        }),
      );
    }
  }

  /// Cancellation shared with asynchronous command startup.
  final cancellation = CancellationToken();

  final _interrupted = Completer<void>();
  final _subscriptions = <StreamSubscription<ProcessSignal>>[];

  /// Completes after the first termination signal.
  Future<void> get interrupted => _interrupted.future;

  /// Cancels every listener, including signals that never arrived.
  Future<void> close() async {
    for (final subscription in _subscriptions) {
      await subscription.cancel();
    }
  }
}

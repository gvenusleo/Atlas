import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/terminal_session.dart';

/// Tracks the live shells of the workspace terminals so the application can
/// release every pseudo-terminal before it exits.
///
/// Terminating the app shuts the Dart isolate down inside `NSApplication
/// terminate`, and pty2 closes the PTY master from a native finalizer at that
/// point. Closing a master whose slave is still held by the running shell
/// blocks the main thread, which freezes the application after the last window
/// closes, so [closeAll] kills the shells while the isolate is still running.
final class TerminalSessionRegistry {
  final _handles = <TerminalHandle>{};

  /// The number of tracked shells.
  int get length => _handles.length;

  /// Whether no shell is tracked.
  bool get isEmpty => _handles.isEmpty;

  /// Starts tracking [handle].
  void track(TerminalHandle handle) => _handles.add(handle);

  /// Stops tracking [handle] without killing it.
  void untrack(TerminalHandle handle) => _handles.remove(handle);

  /// Kills every tracked shell and forgets it.
  ///
  /// Synchronous on purpose: the exit hook that calls this must return even
  /// when a shell never acknowledges its termination. A shell that fails to
  /// die must not keep the remaining ones alive, because every unreleased
  /// pseudo-terminal risks blocking isolate shutdown.
  void closeAll() {
    final handles = List.of(_handles);
    _handles.clear();
    for (final handle in handles) {
      try {
        handle.kill();
      } on Object {
        // Best effort: the process is exiting and cannot report the failure.
      }
    }
  }
}

/// The process-wide registry of live workspace shells.
final terminalSessionRegistryProvider = Provider<TerminalSessionRegistry>(
  (ref) => TerminalSessionRegistry(),
);

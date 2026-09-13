import 'dart:async';
import 'dart:io';

import 'package:pty2/pty2.dart';

/// A shell-backed terminal the application must release before it exits.
///
/// The macOS embedder shuts the Dart isolate down inside `NSApplication
/// terminate`, and pty2 releases the pseudo-terminal from a native finalizer
/// during that shutdown. Closing a PTY master whose slave is still held by a
/// running shell blocks the closing thread, so the exit path has to kill every
/// shell while the isolate is still running.
abstract interface class TerminalHandle {
  /// Kills the shell and releases its pseudo-terminal without awaiting.
  void kill();
}

/// Owns one interactive shell process and its pseudo-terminal resources.
final class TerminalSession implements TerminalHandle {
  /// Whether a shell process is currently attached.
  bool get isRunning => _pty != null;

  /// Shell executable started in the workspace terminal.
  static String get executable => Platform.isWindows
      ? 'cmd.exe'
      : Platform.environment['SHELL'] ?? '/bin/sh';

  /// Login-shell arguments so macOS `path_helper` and profile scripts run.
  ///
  /// Finder-launched apps inherit a minimal PATH. A login shell loads
  /// `/etc/zprofile`, which prepends Homebrew and other `/etc/paths.d` entries.
  static List<String> get arguments =>
      Platform.isWindows ? const <String>[] : const ['-l'];

  PseudoTerminal? _pty;
  StreamSubscription<String>? _outputSubscription;

  /// Starts a shell and forwards output and exit status to the callbacks.
  ///
  /// Ignored while a shell is already attached.
  Future<void> start({
    required String workingDirectory,
    required void Function(String) onOutput,
    required void Function(int) onExit,
  }) async {
    if (_pty != null) {
      return;
    }
    final pty = PseudoTerminal.start(
      executable,
      arguments,
      workingDirectory: workingDirectory,
      environment: const {'TERM': 'xterm-256color'},
    );
    _pty = pty;
    _outputSubscription = pty.out.listen(onOutput);
    unawaited(
      pty.exitCode.then((code) {
        if (identical(_pty, pty)) {
          _pty = null;
          onExit(code);
        }
      }),
    );
  }

  /// Sends emulator input to the shell.
  void write(String data) => _pty?.write(data);

  /// Updates the shell terminal dimensions.
  void resize(int cols, int rows) => _pty?.resize(cols, rows);

  @override
  void kill() {
    _pty?.kill();
    _pty = null;
  }

  /// Stops the shell and releases its stream subscription.
  Future<void> close() async {
    await _outputSubscription?.cancel();
    _outputSubscription = null;
    kill();
  }
}

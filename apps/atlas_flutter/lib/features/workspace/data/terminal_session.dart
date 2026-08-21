import 'dart:async';
import 'dart:io';

import 'package:pty2/pty2.dart';

/// Owns one interactive shell process and its pseudo-terminal resources.
final class TerminalSession {
  PseudoTerminal? _pty;
  StreamSubscription<String>? _outputSubscription;

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

  /// Starts a shell and forwards output and exit status to the callbacks.
  Future<void> start({
    required String workingDirectory,
    required void Function(String) onOutput,
    required void Function(int) onExit,
  }) async {
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

  /// Stops the shell and releases its stream subscription.
  Future<void> close() async {
    await _outputSubscription?.cancel();
    _outputSubscription = null;
    _pty?.kill();
    _pty = null;
  }
}

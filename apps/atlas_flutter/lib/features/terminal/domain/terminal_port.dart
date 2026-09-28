abstract interface class TerminalHandle {
  /// Kills the shell and releases its pseudo-terminal without awaiting.
  void kill();
}

/// Interactive session commands used by the terminal view.
abstract interface class TerminalSessionPort implements TerminalHandle {
  /// Starts the shell and forwards its output and exit status.
  Future<void> start({
    required String workingDirectory,
    required void Function(String) onOutput,
    required void Function(int) onExit,
  });

  /// Sends input to the shell.
  void write(String data);

  /// Resizes the terminal window.
  void resize(int cols, int rows);

  /// Closes the shell and its output subscription.
  Future<void> close();
}

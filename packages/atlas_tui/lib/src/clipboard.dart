import 'dart:convert';
import 'dart:io';

import 'package:nocterm/nocterm.dart';

/// Maximum raw bytes base64-encoded into one OSC 52 sequence.
const maxOsc52Bytes = 100000;

/// Copies [text] to the system clipboard.
///
/// Over SSH the terminal-mediated OSC 52 path is used so the text reaches the
/// local terminal emulator's clipboard rather than the remote machine's. On a
/// local macOS session `pbcopy` is preferred (Apple Terminal does not answer
/// OSC 52), falling back to OSC 52 for other terminals. Failures are silent:
/// the user keeps the selection highlight either way.
Future<void> copyToClipboard(
  String text, {
  Terminal? terminal,
  bool Function()? isSsh,
}) async {
  if (text.isEmpty) {
    return;
  }
  final ssh = (isSsh ?? _isSshSession)();
  if (!ssh && Platform.isMacOS) {
    final process = await Process.start('pbcopy', const []);
    process.stdin.write(text);
    await process.stdin.close();
    final exitCode = await process.exitCode;
    if (exitCode == 0) {
      return;
    }
  }
  final resolvedTerminal = terminal ?? _terminalFromBinding();
  if (resolvedTerminal == null) {
    return;
  }
  final encoded = base64Encode(utf8.encode(text));
  if (encoded.length > maxOsc52Bytes) {
    return;
  }
  resolvedTerminal
    ..write('\x1b]52;c;$encoded\x07')
    ..flush();
}

bool _isSshSession() =>
    Platform.environment['SSH_TTY'] != null ||
    Platform.environment['SSH_CONNECTION'] != null;

Terminal? _terminalFromBinding() {
  final binding = NoctermBinding.instance;
  return binding is TerminalBinding ? binding.terminal : null;
}

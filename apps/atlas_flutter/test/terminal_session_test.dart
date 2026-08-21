import 'dart:io';

import 'package:atlas_flutter/features/workspace/data/terminal_session.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('starts a login shell on Unix', () {
    if (Platform.isWindows) {
      expect(TerminalSession.executable, 'cmd.exe');
      expect(TerminalSession.arguments, isEmpty);
      return;
    }
    expect(
      TerminalSession.executable,
      Platform.environment['SHELL'] ?? '/bin/sh',
    );
    expect(TerminalSession.arguments, ['-l']);
  });
}

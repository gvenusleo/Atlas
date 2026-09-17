import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';

/// Exercises missing cleanup utilities in an isolated environment.
Future<void> main() async {
  final watchdog = Timer(const Duration(seconds: 8), () => exit(99));
  try {
    String quote(String value) => "'${value.replaceAll("'", "'\"'\"'")}'";
    final child = Platform.script.resolve('shell_child.dart').toFilePath();
    final result = await ShellTool().execute(
      ToolContext(
        sessionId: SessionId('probe'),
        turnId: TurnId('probe'),
        workingDirectory: Directory.current.path,
      ),
      {
        'command':
            'exec ${quote(Platform.resolvedExecutable)} ${quote(child)} wait',
        'timeout_seconds': 1,
      },
    );
    stdout.write(
      jsonEncode({'content': result.content, 'metadata': result.metadata}),
    );
  } finally {
    watchdog.cancel();
  }
}

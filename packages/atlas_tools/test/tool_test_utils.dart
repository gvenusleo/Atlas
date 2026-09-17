import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

/// Creates an isolated temporary directory for one test.
Future<Directory> tempDir() async {
  final base = await Directory.systemTemp.createTemp('atlas_tools_test_');
  addTearDown(() => base.delete(recursive: true));
  return base;
}

/// Builds a tool context rooted at [dir].
ToolContext toolContext(
  Directory dir, {
  CancellationToken? cancellation,
  void Function(ToolOutputSnapshot)? onOutput,
}) => ToolContext(
  sessionId: SessionId('session-test'),
  turnId: TurnId('turn-test'),
  workingDirectory: dir.path,
  cancellation: cancellation,
  onOutput: onOutput,
);

/// Builds a platform-quoted command for the finite Dart shell fixture.
String shellChildCommand(String mode) {
  String quote(String value) => Platform.isWindows
      ? "'${value.replaceAll("'", "''")}'"
      : "'${value.replaceAll("'", "'\"'\"'")}'";
  final executable = quote(Platform.resolvedExecutable);
  final fixture = quote(File('test/fixtures/shell_child.dart').absolute.path);
  return '${Platform.isWindows ? '& ' : ''}$executable $fixture $mode';
}

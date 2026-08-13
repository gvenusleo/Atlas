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
ToolContext toolContext(Directory dir, {CancellationToken? cancellation}) =>
    ToolContext(
      sessionId: SessionId('session-test'),
      turnId: TurnId('turn-test'),
      workingDirectory: dir.path,
      cancellation: cancellation,
    );

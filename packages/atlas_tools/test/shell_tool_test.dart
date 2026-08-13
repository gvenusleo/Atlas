import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = ShellTool();

  test('runs a command and returns its output', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'command': 'echo hello',
    });

    expect(result.isError, isFalse);
    expect(result.content, contains('hello'));
    expect(result.metadata['exit_code'], 0);
  });

  test('passes stdin to the command', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'command': 'cat',
      'stdin': 'from stdin',
    });

    expect(result.isError, isFalse);
    expect(result.content, contains('from stdin'));
  });

  test('reports a non-zero exit code in the result', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {'command': 'exit 3'});

    expect(result.isError, isFalse);
    expect(result.content, contains('(exit code: 3)'));
    expect(result.metadata['exit_code'], 3);
  });

  test('reports a missing command as a non-zero exit code', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'command': 'kkjh34234-not-a-command',
    });

    // The shell starts successfully; the missing command is its exit code.
    expect(result.isError, isFalse);
    expect(result.content, contains('(exit code:'));
    expect(result.metadata['exit_code'], isNot(0));
  });

  test('times out and kills the command', () async {
    final dir = await tempDir();

    final stopwatch = Stopwatch()..start();
    final result = await tool.execute(toolContext(dir), {
      'command': 'sleep 5',
      'timeout_seconds': 1,
    });
    stopwatch.stop();

    expect(result.isError, isTrue);
    expect(result.content, contains('timed out'));
    expect(stopwatch.elapsed, lessThan(const Duration(seconds: 3)));
  });

  test('kills the whole command tree on timeout', () async {
    final dir = await tempDir();

    // `sleep 5 & wait` runs the child in the background, so killing the shell
    // alone would leave the child holding the output pipes until it finishes.
    final stopwatch = Stopwatch()..start();
    final result = await tool.execute(toolContext(dir), {
      'command': 'sleep 5 & wait',
      'timeout_seconds': 1,
    });
    stopwatch.stop();

    expect(result.isError, isTrue);
    expect(result.content, contains('timed out'));
    expect(stopwatch.elapsed, lessThan(const Duration(seconds: 3)));
  });

  test('cancellation kills the command', () async {
    final dir = await tempDir();
    final cancellation = CancellationToken();

    final run = tool.execute(toolContext(dir, cancellation: cancellation), {
      'command': 'sleep 5',
    });
    unawaited(
      Future<void>.delayed(
        const Duration(milliseconds: 100),
        cancellation.cancel,
      ),
    );
    final result = await run;

    expect(result.isError, isTrue);
    expect(result.content, contains('cancelled'));
  });

  test('truncates very large output keeping head and tail', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'command': 'seq 1 100000',
    });

    expect(result.isError, isFalse);
    expect(result.metadata['truncated'], isTrue);
    expect(result.content, contains('[output truncated]'));
    expect(result.content, contains('1'));
    expect(result.content, contains('100000'));
  });

  test('rejects an empty command', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {'command': '   '});

    expect(result.isError, isTrue);
  });
}

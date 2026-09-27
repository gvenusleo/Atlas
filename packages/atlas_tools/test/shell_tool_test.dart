import 'dart:async';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = ShellTool();

  test(
    'copies child environment overrides and preserves inherited variables',
    () async {
      final dir = await tempDir();
      final values = {'ATLAS_SHELL_ENV_TEST': 'initial'};
      final configured = ShellTool(environment: values);
      values['ATLAS_SHELL_ENV_TEST'] = 'mutated';
      final command = Platform.isWindows
          ? r'Write-Output $env:ATLAS_SHELL_ENV_TEST; Write-Output $env:PATH'
          : r'printf "%s\n%s" "$ATLAS_SHELL_ENV_TEST" "$PATH"';
      final result = await configured.execute(toolContext(dir), {
        'command': command,
      });
      expect(result.metadata['exit_code'], 0);
      expect(result.content, startsWith('initial'));
      expect(result.content, contains(Platform.environment['PATH']!));
    },
  );

  test('times out and kills the command', () async {
    final dir = await tempDir();

    final stopwatch = Stopwatch()..start();
    final result = await tool.execute(toolContext(dir), {
      'command': shellChildCommand('wait'),
      'timeout_seconds': 1,
    });
    stopwatch.stop();

    expect(result.isError, isTrue);
    expect(result.content, contains('timed out'));
    expect(stopwatch.elapsed, lessThan(const Duration(seconds: 3)));
  });

  test('kills the whole command tree on timeout', () async {
    final dir = await tempDir();

    // The fixture waits for a child that inherits its output pipes.
    final stopwatch = Stopwatch()..start();
    final result = await tool.execute(toolContext(dir), {
      'command': shellChildCommand('tree'),
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
      'command': shellChildCommand('wait'),
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
      'command': shellChildCommand('numbers'),
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

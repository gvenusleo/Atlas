import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_tools/atlas_tools.dart';
import 'package:atlas_tools/src/shell_output.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = ShellTool();

  test('resolves relative cwd against the session directory', () async {
    final dir = await tempDir();
    final sub = await Directory('${dir.path}/sub').create();
    final result = await tool.execute(toolContext(dir), {
      'command': shellChildCommand('cwd'),
      'cwd': 'sub',
    });
    expect(result.isError, isFalse);
    expect(result.content, sub.path);
  });

  test('allows an explicit timeout longer than five minutes', () async {
    final result = await tool.execute(toolContext(await tempDir()), {
      'command': shellChildCommand('exit'),
      'timeout_seconds': 600,
    });
    expect(result.isError, isFalse);
    expect(result.metadata['exit_code'], 3);
  });

  test('contains malformed UTF-8 without unhandled errors', () async {
    final result = await tool.execute(toolContext(await tempDir()), {
      'command': shellChildCommand('invalid'),
    });
    expect(result.isError, isFalse);
    expect(result.content, contains('\uFFFD'));
  });

  test('bounds Unicode output by UTF-8 bytes', () async {
    final result = await tool.execute(toolContext(await tempDir()), {
      'command': shellChildCommand('flood'),
    });
    expect(result.isError, isFalse);
    expect(result.metadata['truncated'], isTrue);
    expect(utf8.encode(result.content).length, lessThanOrEqualTo(50 * 1024));
    expect(result.content, isNot(contains('\uFFFD')));
  });

  test(
    'streams combined output before completion then sends the final snapshot',
    () async {
      final outputs = <ToolOutputSnapshot>[];
      final first = Completer<void>();
      var finished = false;
      final run = tool
          .execute(
            toolContext(
              await tempDir(),
              onOutput: (output) {
                outputs.add(output);
                if (!first.isCompleted) first.complete();
              },
            ),
            {'command': shellChildCommand('output')},
          )
          .then((result) {
            finished = true;
            return result;
          });
      await first.future.timeout(const Duration(seconds: 3));
      expect(finished, isFalse);
      expect(outputs.first.content, 'first\n');
      final result = await run;
      expect(result.content, 'first\nsecond\n');
      expect(outputs.last.content, result.content);
      expect(outputs.last.totalBytes, 13);
    },
  );

  test('drains a MiB input echo concurrently with input writes', () async {
    final result = await tool.execute(toolContext(await tempDir()), {
      'command': shellChildCommand('echo'),
      'stdin': 'x' * (1024 * 1024),
      'timeout_seconds': 4,
    });
    expect(result.isError, isFalse);
    expect(result.metadata['total_bytes'], 1024 * 1024);
    expect(result.metadata['truncated'], isTrue);
  });

  test(
    'reports a child closing stdin early without unhandled pipe errors',
    () async {
      final result = await tool.execute(toolContext(await tempDir()), {
        'command': shellChildCommand('closed_input'),
        'stdin': 'x' * (4 * 1024 * 1024),
        'timeout_seconds': 4,
      });
      // Dart may destroy the stdin sink on process exit before an OS pipe error
      // arrives. Either outcome must finish cleanly and retain captured output.
      if (result.isError) {
        expect(result.metadata['termination_reason'], 'stdin_failed');
      }
      expect(result.content, contains('done without reading input'));
    },
  );

  test(
    'contains a broken input pipe while the command is still running',
    () async {
      final result = await tool.execute(toolContext(await tempDir()), {
        'command': 'exec 0<&-; echo closed; sleep 1',
        'stdin': 'x' * (4 * 1024 * 1024),
      });
      expect(result.isError, isTrue);
      expect(result.metadata['termination_reason'], 'stdin_failed');
      expect(result.content, contains('closed'));
    },
    skip: Platform.isWindows ? 'POSIX file-descriptor closure' : false,
  );

  test('handles split Unicode and removes split terminal controls', () async {
    final context = toolContext(await tempDir());
    final unicode = await tool.execute(context, {
      'command': shellChildCommand('unicode'),
    });
    expect(unicode.content, '你好🙂');
    final controls = await tool.execute(context, {
      'command': shellChildCommand('controls'),
    });
    expect(controls.content, 'beforeredafter\n');
  });

  test('keeps already captured text when cancelled', () async {
    final cancellation = CancellationToken();
    final context = toolContext(
      await tempDir(),
      cancellation: cancellation,
      onOutput: (_) => cancellation.cancel(),
    );
    final result = await tool.execute(context, {
      'command': shellChildCommand('wait'),
    });
    expect(result.isError, isTrue);
    expect(result.content, contains('ready'));
    expect(result.metadata['termination_reason'], 'cancelled');
  });

  test(
    'does not launch pre-cancelled commands or react to late cancellation',
    () async {
      final cancellation = CancellationToken()..cancel();
      final result = await tool.execute(
        toolContext(await tempDir(), cancellation: cancellation),
        {'command': shellChildCommand('output')},
      );
      expect(result.metadata['total_bytes'], 0);
      expect(result.metadata['termination_reason'], 'cancelled');
      final late = CancellationToken();
      final completed = await tool.execute(
        toolContext(await tempDir(), cancellation: late),
        {'command': shellChildCommand('unicode')},
      );
      late.cancel();
      await Future<void>.delayed(Duration.zero);
      expect(completed.isError, isFalse);
    },
  );

  test(
    'reports a safe start failure and accepts directories outside the session',
    () async {
      final dir = await tempDir();
      final other = await tempDir();
      final valid = await tool.execute(toolContext(dir), {
        'command': shellChildCommand('cwd'),
        'cwd': other.path,
      });
      expect(valid.content, other.path);
      final failed = await tool.execute(toolContext(dir), {
        'command': 'PRIVATE_COMMAND_MARKER',
        'cwd': '${dir.path}/missing',
      });
      expect(failed.isError, isTrue);
      expect(failed.content, contains('could not start'));
      expect(failed.content, isNot(contains('PRIVATE_COMMAND_MARKER')));
      expect(failed.content, isNot(contains('ProcessException')));
    },
  );

  test('validates argument types and timeout precision', () async {
    final context = toolContext(await tempDir());
    for (final args in <JsonObject>[
      {'command': 42},
      {'command': 'x', 'stdin': []},
      {'command': 'x', 'cwd': 3},
      {'command': 'x', 'timeout_seconds': 1.5},
      {'command': 'x', 'timeout_seconds': 0},
    ]) {
      expect((await tool.execute(context, args)).isError, isTrue);
    }
  });

  test('rejects capacities too small for truncation text at construction', () {
    for (final limit in [-1, 0, 20, 28, 54]) {
      expect(() => ShellOutputBuffer(limit), throwsArgumentError);
    }
    final buffer = ShellOutputBuffer(55)..add('ABCDEFGHIJKLMNOPQRSTUVWXYZ' * 4);
    expect(utf8.encode(buffer.text).length, lessThanOrEqualTo(55));
    expect(buffer.text, contains('[output truncated]'));
  });

  test(
    'bounds retention throughout a large stream, including character edges',
    () {
      final buffer = ShellOutputBuffer(50 * 1024);
      for (var i = 0; i < 100; i++) {
        buffer.add('汉🙂' * 3000);
        expect(buffer.retainedBytes, lessThanOrEqualTo(50 * 1024));
        expect(utf8.encode(buffer.text).length, lessThanOrEqualTo(50 * 1024));
        expect(buffer.text, isNot(contains('\uFFFD')));
      }
      expect(buffer.truncated, isTrue);
    },
  );

  test(
    'rejects oversized PID lists without signalling an unrelated process',
    () async {
      final dir = await tempDir();
      final guard = await Process.start('/bin/sleep', ['20']);
      var guardExited = false;
      final guardExit = guard.exitCode.then((_) => guardExited = true);
      addTearDown(() async {
        guard.kill(ProcessSignal.sigkill);
        await guardExit;
      });
      // Byte truncation turns the suffix of a nonexistent PID into the guard's
      // real PID. Verify the fixture cannot address any other live process.
      final payload =
          '${'999999999\n' * 6000}'
          '999999999${guard.pid}\n'
          '${'x' * (25572 - '${guard.pid}'.length - 1)}';
      final display = ShellOutputBuffer(shellOutputLimit)..add(payload);
      final parsed = display.text
          .split('\n')
          .map(int.tryParse)
          .whereType<int>()
          .toSet();
      expect(parsed, {999999999, guard.pid});
      final data = await File('${dir.path}/pids').writeAsString(payload);
      final marker = '${dir.path}/called';
      final utility = await File('${dir.path}/pgrep').writeAsString(
        "#!/bin/sh\n"
        "if [ -e '$marker' ]; then exit 1; fi\n"
        ": > '$marker'\n"
        "exec /bin/cat '${data.path}'\n",
      );
      expect(
        (await Process.run('/bin/chmod', ['+x', utility.path])).exitCode,
        0,
      );
      final result = await Process.run(
        Platform.resolvedExecutable,
        [File('test/fixtures/shell_cleanup_probe.dart').absolute.path],
        environment: {'PATH': dir.path},
      );
      expect(result.exitCode, 0);
      final output = jsonDecode(result.stdout as String) as Map;
      expect(
        guardExited,
        isFalse,
        reason: 'PID output must never be display-truncated',
      );
      expect((output['metadata'] as Map)['cleanup_failed'], isTrue);
      expect((output['metadata'] as Map)['termination_reason'], 'timed_out');
    },
    skip: !Platform.isLinux
        ? 'Linux PID range and isolated POSIX utilities'
        : false,
  );

  test(
    'reports unavailable cleanup utilities and still kills the root process',
    () async {
      final clock = Stopwatch()..start();
      final result = await Process.run(
        Platform.resolvedExecutable,
        [File('test/fixtures/shell_cleanup_probe.dart').absolute.path],
        environment: {'PATH': (await tempDir()).path},
      );
      expect(result.exitCode, 0);
      final output = jsonDecode(result.stdout as String) as Map;
      final metadata = output['metadata'] as Map;
      expect(metadata['cleanup_failed'], isTrue);
      expect(metadata['termination_reason'], 'timed_out');
      expect(metadata['exit_code'], isNotNull);
      expect(
        output['content'],
        contains('process cleanup could not be confirmed'),
      );
      expect(clock.elapsed, lessThan(const Duration(seconds: 5)));
    },
    skip: Platform.isWindows ? 'POSIX exec and PATH-isolated pgrep' : false,
  );

  test(
    'preserves a real negative exit code without reporting a timeout',
    () async {
      final result = await tool.execute(toolContext(await tempDir()), {
        'command': r'kill -HUP $$',
      });
      expect(result.isError, isFalse);
      expect(result.metadata['exit_code'], -1);
      expect(result.metadata['termination_reason'], isNull);
      expect(result.content, '(exit code: -1)');
    },
    skip: Platform.isWindows ? 'POSIX signal exit status' : false,
  );

  test(
    'times out even when the parent exits but a child holds the pipes',
    () async {
      final clock = Stopwatch()..start();
      int? childPid;
      addTearDown(() {
        if (childPid != null) Process.killPid(childPid!, ProcessSignal.sigkill);
      });
      final result = await tool.execute(
        toolContext(
          await tempDir(),
          onOutput: (output) {
            final match = RegExp(r'child:(\d+)').firstMatch(output.content);
            if (match != null) childPid = int.parse(match[1]!);
          },
        ),
        {'command': shellChildCommand('orphan'), 'timeout_seconds': 1},
      );
      expect(result.isError, isTrue);
      expect(result.metadata['termination_reason'], 'timed_out');
      expect(clock.elapsed, lessThan(const Duration(seconds: 4)));
    },
    skip: Platform.isWindows
        ? 'inheritStdio child orphaning differs on Windows'
        : false,
  );

  test(
    'escalates termination when a child ignores TERM',
    () async {
      int? childPid;
      addTearDown(() {
        if (childPid != null) Process.killPid(childPid!, ProcessSignal.sigkill);
      });
      final clock = Stopwatch()..start();
      final result = await tool.execute(
        toolContext(
          await tempDir(),
          onOutput: (output) {
            final match = RegExp(r'pid:(\d+)').firstMatch(output.content);
            if (match != null) childPid = int.parse(match[1]!);
          },
        ),
        {'command': shellChildCommand('ignore_term'), 'timeout_seconds': 1},
      );
      expect(childPid, isNotNull);
      expect(result.metadata['termination_reason'], 'timed_out');
      expect(clock.elapsed, lessThan(const Duration(seconds: 5)));
    },
    skip: Platform.isWindows
        ? 'Windows does not have POSIX TERM handlers'
        : false,
  );
}

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

/// The default shell timeout in seconds.
const defaultShellTimeoutSeconds = 30;

/// The maximum shell timeout in seconds.
const maxShellTimeoutSeconds = 300;

/// The maximum command output returned to the model.
const shellOutputLimit = 50 * 1024;

/// The number of leading and trailing code units kept when output is truncated.
const shellOutputEdge = shellOutputLimit ~/ 2;

/// Runs a command with the platform default shell.
final class ShellTool implements Tool {
  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'shell',
    description:
        'Run a command in the session working directory, optionally pass '
        'text to standard input, and return combined output.',
    inputSchema: {
      'type': 'object',
      'properties': {
        'command': {
          'type': 'string',
          'description': 'Command to execute with the platform default shell.',
        },
        'stdin': {
          'type': 'string',
          'description':
              'Optional text passed unchanged to the command\'s '
              'standard input.',
        },
        'cwd': {
          'type': 'string',
          'description':
              'Working directory override. Omit to use the '
              'session working directory.',
        },
        'timeout_seconds': {
          'type': 'integer',
          'description': 'Optional timeout in seconds.',
        },
      },
      'required': ['command'],
    },
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    final command = arguments['command'] as String? ?? '';
    if (command.trim().isEmpty) {
      return ToolResult(
        content: 'command must be a non-empty string',
        isError: true,
      );
    }
    final timeoutSeconds = (arguments['timeout_seconds'] as num?)?.toInt();
    if (timeoutSeconds != null &&
        (timeoutSeconds < 1 || timeoutSeconds > maxShellTimeoutSeconds)) {
      return ToolResult(
        content:
            'timeout_seconds must be between 1 and '
            '$maxShellTimeoutSeconds',
        isError: true,
      );
    }
    final cwd = (arguments['cwd'] as String?)?.trim();
    final workingDirectory = cwd == null || cwd.isEmpty
        ? context.workingDirectory
        : Directory(cwd).absolute.path;
    if (!_allowedDirectory(context, workingDirectory)) {
      return ToolResult(
        content:
            'cwd must be the session working directory or an authorized additional directory',
        isError: true,
      );
    }
    final timeout = Duration(
      seconds: timeoutSeconds ?? defaultShellTimeoutSeconds,
    );

    try {
      final process = await Process.start(
        Platform.isWindows ? 'powershell' : '/bin/sh',
        Platform.isWindows ? ['-Command', command] : ['-c', command],
        workingDirectory: workingDirectory,
      );
      final stdinData = arguments['stdin'] as String?;
      if (stdinData != null && stdinData.isNotEmpty) {
        process.stdin.write(stdinData);
      }
      await process.stdin.close();

      // Drain both pipes in arrival order while the process runs. Register
      // the completion futures before the process can close the streams, so
      // their onDone handlers are captured.
      final combined = StringBuffer();
      final stdoutSub = process.stdout
          .transform(utf8.decoder)
          .listen(combined.write);
      final stderrSub = process.stderr
          .transform(utf8.decoder)
          .listen(combined.write);
      final stdoutDone = stdoutSub.asFuture<void>();
      final stderrDone = stderrSub.asFuture<void>();
      final exit = await _waitForExit(process, timeout, context.cancellation);
      await stdoutDone;
      await stderrDone;
      final bounded = _bounded(combined.toString());
      if (exit == _timedOut) {
        return ToolResult(
          content:
              'command timed out after ${timeout.inSeconds}s\n'
              '${bounded.text}',
          isError: true,
        );
      }
      if (exit == _cancelled) {
        return ToolResult(content: 'command cancelled', isError: true);
      }
      return ToolResult(
        content: exit == 0
            ? bounded.text
            : '${bounded.text}\n(exit code: $exit)',
        metadata: {
          'exit_code': exit,
          'truncated': bounded.truncated,
          'total_bytes': utf8.encode(combined.toString()).length,
        },
      );
    } catch (error) {
      return ToolResult(
        content: error is FormatException ? error.message : '$error',
        isError: true,
      );
    }
  }

  static const _timedOut = -1;
  static const _cancelled = -2;

  /// Completes with the process exit code, killing the process on timeout or
  /// cancellation. The process is escalated to SIGKILL when it ignores the
  /// initial termination signal.
  static Future<int> _waitForExit(
    Process process,
    Duration timeout,
    CancellationToken? cancellation,
  ) {
    final completer = Completer<int>();
    var done = false;
    Timer? timer;
    void finish(int code) {
      if (done) {
        return;
      }
      done = true;
      timer?.cancel();
      completer.complete(code);
    }

    void interrupt() {
      // Windows: `taskkill /T` terminates the whole tree.
      if (Platform.isWindows) {
        unawaited(
          Process.run('taskkill', ['/PID', '${process.pid}', '/T', '/F']),
        );
        process.kill();
      } else {
        // Collect descendants before the shell dies: once the shell exits its
        // children are reparented and `pgrep -P` can no longer find them.
        // Killing the shell alone would leave children (e.g. a backgrounded
        // `sleep`) running as orphans that keep the output pipes open until
        // they finish.
        unawaited(() async {
          final children = await _collectDescendants(process.pid);
          process.kill();
          for (final child in children) {
            Process.killPid(child, ProcessSignal.sigkill);
          }
        }());
      }
      unawaited(
        process.exitCode
            .timeout(
              const Duration(seconds: 2),
              onTimeout: () {
                if (!Platform.isWindows) {
                  process.kill(ProcessSignal.sigkill);
                }
                return -1;
              },
            )
            .catchError((_) => -1),
      );
    }

    process.exitCode.then(finish);
    timer = Timer(timeout, () {
      interrupt();
      finish(_timedOut);
    });
    cancellation?.whenCancelled.then((_) {
      interrupt();
      finish(_cancelled);
    });
    return completer.future;
  }

  /// Collects the process ids of every descendant of [rootPid] by walking
  /// `pgrep -P` parent relationships breadth-first.
  static Future<List<int>> _collectDescendants(int rootPid) async {
    final descendants = <int>[];
    final queue = <int>[rootPid];
    while (queue.isNotEmpty) {
      final parent = queue.removeLast();
      final result = await Process.run('pgrep', ['-P', '$parent']);
      if (result.exitCode != 0) {
        continue;
      }
      final stdout = result.stdout;
      if (stdout is! String) {
        continue;
      }
      for (final line in stdout.split('\n')) {
        final child = int.tryParse(line.trim());
        if (child != null) {
          descendants.add(child);
          queue.add(child);
        }
      }
    }
    return descendants;
  }

  static ({String text, bool truncated}) _bounded(String output) {
    if (output.codeUnits.length <= shellOutputLimit) {
      return (text: output, truncated: false);
    }
    final head = output.substring(0, shellOutputEdge);
    final tail = output.substring(output.length - shellOutputEdge);
    return (text: '$head\n... [output truncated] ...\n$tail', truncated: true);
  }

  /// Checks that an explicit shell directory is one of the session roots.
  static bool _allowedDirectory(ToolContext context, String path) {
    final target = Directory(path).absolute.path;
    final roots = [
      context.workingDirectory,
      ...context.additionalDirectories,
    ].map((root) => Directory(root).absolute.path);
    return roots.any(
      (root) =>
          target == root || target.startsWith('$root${Platform.pathSeparator}'),
    );
  }
}

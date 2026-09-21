import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:atlas_runtime/atlas_runtime.dart';

import 'file_path.dart';
import 'shell_output.dart';

/// The default shell timeout in seconds.
const defaultShellTimeoutSeconds = 30;

/// Maximum UTF-8 bytes of captured text, including its truncation marker.
const shellOutputLimit = 50 * 1024;

/// Runs a command through /bin/sh on Unix or PowerShell on Windows.
final class ShellTool implements Tool {
  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'shell',
    description:
        'Run a command through /bin/sh on Unix or PowerShell on Windows. '
        'Optionally pass stdin once; return combined output and exit status.',
    inputSchema: {
      'type': 'object',
      'properties': {
        'command': {
          'type': 'string',
          'description': 'Shell command to execute.',
        },
        'stdin': {
          'type': 'string',
          'description':
              'Optional text written once, followed by end of input.',
        },
        'cwd': {
          'type': 'string',
          'description':
              'Absolute directory or path relative to the session directory. '
              'Defaults to the session directory.',
        },
        'timeout_seconds': {
          'type': 'integer',
          'minimum': 1,
          'description':
              'Execution timeout in seconds. Defaults to 30; '
              'set a longer timeout for builds and other long commands.',
        },
      },
      'required': ['command'],
    },
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    final command = arguments['command'];
    final input = arguments['stdin'];
    final cwd = arguments['cwd'];
    final seconds = arguments['timeout_seconds'] ?? defaultShellTimeoutSeconds;
    if (command is! String || command.trim().isEmpty) {
      return _invalid('command must be a non-empty string');
    }
    if (input != null && input is! String) {
      return _invalid('stdin must be a string');
    }
    if (cwd != null && cwd is! String) return _invalid('cwd must be a string');
    // Duration stores microseconds in a signed native integer.
    if (seconds is! int ||
        seconds < 1 ||
        seconds > 0x7fffffffffffffff ~/ 1000000) {
      return _invalid(
        'timeout_seconds must be a positive representable integer',
      );
    }
    final workingDirectory = cwd == null || (cwd as String).trim().isEmpty
        ? context.workingDirectory
        : resolveFilePath(context.workingDirectory, cwd);
    return _ShellExecution(
      context,
      Duration(seconds: seconds),
    ).run(command, workingDirectory, input as String?);
  }

  static ToolResult _invalid(String message) =>
      ToolResult(content: message, isError: true);
}

final class _ShellExecution(final ToolContext context, final Duration timeout) {
  final _buffer = ShellOutputBuffer(shellOutputLimit);
  final _completed = Completer<void>();
  final _interrupted = Completer<void>();
  final _subscriptions = <StreamSubscription<String>>[];
  Process? _process;
  Timer? _deadline;
  Timer? _drainDeadline;
  Timer? _outputTimer;
  var _closed = false;
  var _stdinDone = false;
  var _pipesDone = 0;
  var _totalBytes = 0;
  var _dirty = false;
  var _published = false;
  int? _exitCode;
  String? _reason;
  bool _cleanupFailed = false;

  Future<ToolResult> run(String command, String cwd, String? input) async {
    if (context.cancellation?.isCancelled == true) {
      _stop('cancelled');
      return _result();
    }
    _deadline = Timer(timeout, () => _stop('timed_out'));
    final cancellationSubscription = context.cancellation?.whenCancelled
        .asStream()
        .listen((_) {
          if (!_closed) _stop('cancelled');
        });
    try {
      final process = await Process.start(
        Platform.isWindows ? 'powershell' : '/bin/sh',
        Platform.isWindows ? ['-Command', command] : ['-c', command],
        workingDirectory: cwd,
      );
      _process = process;
      _listen(process.stdout);
      _listen(process.stderr);
      unawaited(
        process.exitCode.then((code) {
          _exitCode = code;
          _checkComplete();
          if (!_completed.isCompleted && !_closed) {
            _drainDeadline = Timer(
              const Duration(seconds: 2),
              () => _stop('output_incomplete'),
            );
          }
        }, onError: (Object _) => _stop('io_failed')),
      );
      // Readers and error handlers are installed before any potentially blocking
      // input flush. Input/output must progress concurrently.
      unawaited(_writeInput(process, input));
      await Future.any([_completed.future, _interrupted.future]);
      _deadline?.cancel();
      _drainDeadline?.cancel();
      if (_reason != null) await _terminate();
    } catch (_) {
      _stop(_process == null ? 'start_failed' : 'io_failed');
      if (_process != null) await _terminate();
    } finally {
      _closed = true;
      await cancellationSubscription?.cancel();
      _deadline?.cancel();
      _drainDeadline?.cancel();
      _outputTimer?.cancel();
      // A descendant can keep a pipe open even after the shell has exited.
      // Cancelling subscriptions must not make completion unbounded again.
      await Future.wait(_subscriptions.map((sub) => sub.cancel()))
          .timeout(const Duration(seconds: 1), onTimeout: () => <void>[]);
      _publish();
    }
    return _result();
  }

  void _listen(Stream<List<int>> bytes) {
    final filter = ShellTextFilter();
    final subscription = bytes
        .map((data) {
          _totalBytes += data.length;
          return data;
        })
        .transform(const Utf8Decoder(allowMalformed: true))
        .listen(
          (text) {
            _buffer.add(filter.add(text));
            _dirty = true;
            if (context.onOutput == null) return;
            if (!_published) {
              _publish();
              _published = true;
            }
            _outputTimer ??= Timer(const Duration(milliseconds: 100), () {
              _outputTimer = null;
              _publish();
            });
          },
          onError: (Object _) => _stop('io_failed'),
          onDone: () {
            _pipesDone++;
            _checkComplete();
          },
          cancelOnError: true,
        );
    _subscriptions.add(subscription);
  }

  Future<void> _writeInput(Process process, String? input) async {
    try {
      if (_reason == null && input != null) process.stdin.write(input);
      await process.stdin.close();
    } catch (_) {
      _stop('stdin_failed');
    } finally {
      _stdinDone = true;
      _checkComplete();
    }
  }

  void _checkComplete() {
    if (_exitCode != null &&
        _stdinDone &&
        _pipesDone == 2 &&
        !_completed.isCompleted) {
      _completed.complete();
    }
  }

  void _stop(String reason) {
    if (_closed || _completed.isCompleted || _reason != null) return;
    _reason = reason;
    _interrupted.complete();
  }

  void _publish() {
    if (!_dirty || context.onOutput == null) return;
    _dirty = false;
    context.onOutput!(
      ToolOutputSnapshot(
        content: _buffer.text,
        totalBytes: _totalBytes,
        truncated: _buffer.truncated,
      ),
    );
  }

  Future<void> _terminate() async {
    final process = _process!;
    final children = <int>{};
    // Enumerate while parent relationships still exist. Already reparented
    // descendants cannot be recovered through pgrep and are not claimed killed.
    if (_exitCode == null) {
      try {
        if (Platform.isWindows) {
          final result = await _cleanupCommand('taskkill', [
            '/PID',
            '${process.pid}',
            '/T',
            '/F',
          ]);
          if (result.$1 != 0) _cleanupFailed = true;
        } else {
          final remaining = <int>[process.pid];
          final clock = Stopwatch()..start();
          while (remaining.isNotEmpty) {
            if (clock.elapsedMilliseconds >= 500) {
              _cleanupFailed = true;
              break;
            }
            final parent = remaining.removeLast();
            final result = await _cleanupCommand('pgrep', ['-P', '$parent']);
            if (result.$1 != 0 && result.$1 != 1) _cleanupFailed = true;
            for (final line in result.$2.split('\n')) {
              final pid = int.tryParse(line.trim());
              if (pid != null && children.add(pid)) remaining.add(pid);
            }
          }
        }
      } catch (_) {
        _cleanupFailed = true;
      }
      if (!Platform.isWindows) {
        for (final pid in children) {
          _kill(() => Process.killPid(pid));
        }
      }
      if (_exitCode == null) _kill(process.kill);
      // Exit is not enough: a descendant might ignore TERM and keep a pipe open.
      await _completed.future.timeout(
        const Duration(seconds: 2),
        onTimeout: () {},
      );
      if (!Platform.isWindows) {
        for (final pid in children) {
          _kill(() => Process.killPid(pid, ProcessSignal.sigkill));
        }
      }
      if (_exitCode == null) {
        _kill(() => process.kill(ProcessSignal.sigkill));
      }
    } else if (_pipesDone != 2) {
      _cleanupFailed = true;
    }
    await _completed.future.timeout(
      const Duration(seconds: 1),
      onTimeout: () {
        _cleanupFailed = true;
      },
    );
  }

  void _kill(bool Function() kill) {
    try {
      kill();
    } catch (_) {
      _cleanupFailed = true;
    }
  }

  ToolResult _result() {
    final summary = switch (_reason) {
      'timed_out' => 'command timed out after ${timeout.inSeconds}s',
      'cancelled' => 'command cancelled',
      'start_failed' =>
        'could not start shell command; check the shell and working directory',
      'stdin_failed' => 'command input could not be delivered',
      'output_incomplete' =>
        'command exited but its output pipes did not close',
      'io_failed' => 'command I/O failed',
      _ => '',
    };
    final output = _buffer.text;
    return ToolResult(
      content: [
        if (summary.isNotEmpty) summary,
        if (output.isNotEmpty) output,
        if (_reason == null && _exitCode != 0) '(exit code: $_exitCode)',
        if (_cleanupFailed) 'process cleanup could not be confirmed',
      ].join('\n'),
      isError: _reason != null || _cleanupFailed,
      metadata: {
        if (_exitCode != null) 'exit_code': _exitCode,
        if (_reason != null) 'termination_reason': _reason,
        if (_cleanupFailed) 'cleanup_failed': true,
        'truncated': _buffer.truncated,
        'total_bytes': _totalBytes,
      },
    );
  }
}

/// Runs a cleanup utility with bounded output and its own deadline.
Future<(int, String)> _cleanupCommand(
  String executable,
  List<String> args,
) async {
  final process = await Process.start(executable, args);
  final output = BytesBuilder(copy: false);
  var overflowed = false;
  // This output is machine-readable (including PIDs), so truncating even one
  // digit can change the target of a signal. Reject overflow as a whole.
  final sub = process.stdout.listen((bytes) {
    if (overflowed) return;
    if (output.length + bytes.length > shellOutputLimit) {
      overflowed = true;
    } else {
      output.add(bytes);
    }
  }, onError: (Object _) {});
  final err = process.stderr.listen((_) {}, onError: (Object _) {});
  final drained = Future.wait([sub.asFuture<void>(), err.asFuture<void>()]);
  // Attach a handler before the process can finish or a pipe can fail.
  unawaited(drained.catchError((Object _) => <void>[]));
  unawaited(process.stdin.close().catchError((Object _) {}));
  var exited = false;
  final exitCode = process.exitCode.then((code) {
    exited = true;
    return code;
  });
  try {
    final exit = await exitCode.timeout(const Duration(milliseconds: 500));
    // Pipe delivery can trail the exit notification.
    await drained.timeout(const Duration(milliseconds: 100));
    if (overflowed) throw StateError('cleanup output exceeded its limit');
    return (exit, utf8.decode(output.takeBytes(), allowMalformed: true));
  } finally {
    if (!exited) process.kill(ProcessSignal.sigkill);
    await Future.wait([sub.cancel(), err.cancel()])
        .timeout(const Duration(milliseconds: 100), onTimeout: () => <void>[]);
  }
}

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

/// Safe diagnostic categories; shell output and variable values stay private.
enum ShellEnvironmentFailure {
  /// No usable home directory was inherited.
  missingHome,

  /// The configured shell is not an absolute zsh, bash, or sh executable.
  unsupportedShell,

  /// The shell could not be started.
  startFailed,

  /// The shell exited unsuccessfully.
  nonZeroExit,

  /// Startup or pipe draining exceeded the deadline.
  timedOut,

  /// Combined stdout and stderr exceeded the byte limit.
  outputLimit,

  /// A process pipe failed.
  ioFailed,

  /// The shell did not return a complete environment frame.
  invalidOutput,
}

/// An immutable startup snapshot, with a category when recovery fell back.
final class ShellEnvironment(
  Map<String, String> environment, {

  /// Why the original process environment was retained, if recovery failed.
  final ShellEnvironmentFailure? failure,
}) {
  /// Copies the snapshot so later callers cannot mutate a shared environment.
  this : environment = Map.unmodifiable(environment);

  /// Exported variables used by configuration and local child processes.
  final Map<String, String> environment;
}

/// Resolves the macOS user's interactive login environment once per invocation.
///
/// The application calls this at startup and shares the returned snapshot.
/// Other platforms return their inherited environment without spawning a shell.
/// Failure never imports partial output and never prevents application startup.
Future<ShellEnvironment> resolveShellEnvironment({
  Map<String, String>? environment,
  Duration timeout = const Duration(seconds: 10),
  int maxOutputBytes = 1024 * 1024,
}) async {
  final inherited = Map<String, String>.unmodifiable(
    environment ?? Platform.environment,
  );
  ShellEnvironment fallback(ShellEnvironmentFailure failure) =>
      ShellEnvironment(inherited, failure: failure);
  if (!Platform.isMacOS) return ShellEnvironment(inherited);
  final home = inherited['HOME'];
  if (home == null || home.isEmpty) {
    return fallback(ShellEnvironmentFailure.missingHome);
  }
  final shell = inherited['SHELL'] ?? '/bin/zsh';
  if (!shell.startsWith('/') ||
      !const {'zsh', 'bash', 'sh'}.contains(shell.split('/').last)) {
    return fallback(ShellEnvironmentFailure.unsupportedShell);
  }

  final random = Random.secure();
  final marker = List.generate(
    16,
    (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
  ).join();
  final start = '\u0000$marker:start\u0000';
  final end = '\u0000$marker:end\u0000';
  // Only a generated hex marker enters shell source. Paths and inherited
  // variables are passed through Process.start, never interpolated here.
  final command =
      "printf '\\000%s\\000' '$marker:start'; "
      "/usr/bin/env -0 && printf '\\000%s\\000' '$marker:end'";
  final done = Completer<ShellEnvironmentFailure?>();
  final output = BytesBuilder(copy: false);
  final subscriptions = <StreamSubscription<List<int>>>[];
  Process? process;
  int? exitCode;
  var closedPipes = 0;
  var totalBytes = 0;
  var finished = false;

  void fail(ShellEnvironmentFailure failure) {
    if (!done.isCompleted) done.complete(failure);
  }

  void checkDone() {
    if (exitCode != null && closedPipes == 2 && !done.isCompleted) {
      done.complete(exitCode == 0 ? null : ShellEnvironmentFailure.nonZeroExit);
    }
  }

  void listen(Stream<List<int>> stream, {required bool capture}) {
    subscriptions.add(
      stream.listen(
        (bytes) {
          if (done.isCompleted) return;
          totalBytes += bytes.length;
          if (totalBytes > maxOutputBytes) {
            fail(ShellEnvironmentFailure.outputLimit);
          } else if (capture) {
            output.add(bytes);
          }
        },
        onError: (Object _) => fail(ShellEnvironmentFailure.ioFailed),
        onDone: () {
          closedPipes++;
          checkDone();
        },
      ),
    );
  }

  final deadline = Timer(timeout, () => fail(ShellEnvironmentFailure.timedOut));
  // Cover Process.start with the same deadline as shell initialization. A
  // process that arrives after the deadline must still be terminated.
  unawaited(() async {
    try {
      final child = await Process.start(
        shell,
        ['-i', '-l', '-c', command],
        workingDirectory: home,
        environment: inherited,
        includeParentEnvironment: false,
      );
      process = child;
      if (finished || done.isCompleted) {
        await _terminateProbe(child);
        unawaited(child.stdin.close().catchError((Object _) {}));
        unawaited(
          child.stdout.listen((_) {}).cancel().catchError((Object _) {}),
        );
        unawaited(
          child.stderr.listen((_) {}).cancel().catchError((Object _) {}),
        );
        return;
      }
      listen(child.stdout, capture: true);
      listen(child.stderr, capture: false);
      unawaited(child.stdin.close().catchError((Object _) {}));
      unawaited(
        child.exitCode.then((code) {
          exitCode = code;
          if (code != 0) fail(ShellEnvironmentFailure.nonZeroExit);
          checkDone();
        }, onError: (Object _) => fail(ShellEnvironmentFailure.ioFailed)),
      );
    } on Object {
      fail(ShellEnvironmentFailure.startFailed);
    }
  }());

  final failure = await done.future;
  finished = true;
  deadline.cancel();
  if (exitCode == null && process != null) await _terminateProbe(process!);
  // Startup scripts can leave descendants holding the output pipes. Neither
  // their EOF nor stream cancellation may extend the startup wait indefinitely.
  await Future.wait(subscriptions.map((subscription) => subscription.cancel()))
      .timeout(const Duration(milliseconds: 200), onTimeout: () => <void>[]);
  if (failure != null) return fallback(failure);

  try {
    // Locate ASCII framing in raw bytes so non-UTF-8 startup banners outside
    // the frame cannot corrupt a valid UTF-8 environment payload.
    final text = latin1.decode(output.takeBytes());
    final begin = text.indexOf(start);
    final finish = text.indexOf(end, begin < 0 ? 0 : begin + start.length);
    if (begin < 0 || finish < 0) {
      return fallback(ShellEnvironmentFailure.invalidOutput);
    }
    final entries = utf8
        .decode(latin1.encode(text.substring(begin + start.length, finish)))
        .split('\u0000');
    if (entries.length < 2 || entries.removeLast().isNotEmpty) {
      return fallback(ShellEnvironmentFailure.invalidOutput);
    }
    final resolved = Map<String, String>.of(inherited);
    for (final entry in entries) {
      final equals = entry.indexOf('=');
      if (equals <= 0) return fallback(ShellEnvironmentFailure.invalidOutput);
      resolved[entry.substring(0, equals)] = entry.substring(equals + 1);
    }
    for (final key in const ['PWD', 'OLDPWD', 'SHLVL', '_']) {
      if (inherited.containsKey(key)) {
        resolved[key] = inherited[key]!;
      } else {
        resolved.remove(key);
      }
    }
    return ShellEnvironment(resolved);
  } on FormatException {
    return fallback(ShellEnvironmentFailure.invalidOutput);
  }
}

/// Freezes the root before enumerating descendants so killing a foreground
/// command cannot resume the startup script and launch its next command.
Future<void> _terminateProbe(Process process) async {
  try {
    if (!process.kill(ProcessSignal.sigstop)) return;
    final parents = await _processParents();
    if (parents == null) return;
    final descendants = <int>[];
    final pending = <int>[process.pid];
    final seen = <int>{process.pid};
    while (pending.isNotEmpty) {
      final parent = pending.removeLast();
      for (final child in parents[parent] ?? const <int>[]) {
        if (child <= 1 || child == pid || !seen.add(child)) continue;
        descendants.add(child);
        pending.add(child);
      }
    }
    // Preserve the snapshot's parent links until every discovered process is
    // stopped, then kill leaves before parents. Never signal a process group
    // inherited from the application or a PID outside this root's subtree.
    for (final child in descendants) {
      Process.killPid(child, ProcessSignal.sigstop);
    }
    for (final child in descendants.reversed) {
      Process.killPid(child, ProcessSignal.sigkill);
    }
  } on Object {
    // Enumeration can fail; killing the root and returning must still work.
  } finally {
    process.kill(ProcessSignal.sigkill);
    await process.exitCode
        .timeout(const Duration(milliseconds: 200), onTimeout: () => -1)
        .catchError((Object _) => -1);
  }
}

/// Takes one bounded process-table snapshot using an absolute system utility.
/// A partial or malformed snapshot is rejected rather than used as PID input.
Future<Map<int, List<int>>?> _processParents() async {
  final complete = Completer<bool>();
  final output = BytesBuilder(copy: false);
  final subscriptions = <StreamSubscription<List<int>>>[];
  Process? process;
  var closedPipes = 0;
  int? exitCode;
  var totalBytes = 0;
  void finish(bool success) {
    if (!complete.isCompleted) complete.complete(success);
  }

  void checkDone() {
    if (closedPipes == 2 && exitCode != null) finish(exitCode == 0);
  }

  void listen(Stream<List<int>> stream, {required bool capture}) {
    subscriptions.add(
      stream.listen(
        (bytes) {
          if (complete.isCompleted) return;
          totalBytes += bytes.length;
          if (totalBytes > 1024 * 1024) {
            finish(false);
          } else if (capture) {
            output.add(bytes);
          }
        },
        onError: (Object _) => finish(false),
        onDone: () {
          closedPipes++;
          checkDone();
        },
      ),
    );
  }

  final deadline = Timer(
    const Duration(milliseconds: 500),
    () => finish(false),
  );
  unawaited(() async {
    try {
      final child = await Process.start('/bin/ps', ['-axo', 'pid=,ppid=']);
      process = child;
      listen(child.stdout, capture: true);
      listen(child.stderr, capture: false);
      unawaited(child.stdin.close().catchError((Object _) {}));
      unawaited(
        child.exitCode.then((code) {
          exitCode = code;
          checkDone();
        }, onError: (Object _) => finish(false)),
      );
      if (complete.isCompleted) {
        child.kill(ProcessSignal.sigkill);
        for (final subscription in subscriptions) {
          unawaited(subscription.cancel().catchError((Object _) {}));
        }
      }
    } on Object {
      finish(false);
    }
  }());
  final success = await complete.future;
  deadline.cancel();
  if (exitCode == null) process?.kill(ProcessSignal.sigkill);
  await Future.wait(subscriptions.map((subscription) => subscription.cancel()))
      .timeout(const Duration(milliseconds: 100), onTimeout: () => <void>[]);
  if (!success) return null;
  final parents = <int, List<int>>{};
  for (final line in latin1.decode(output.takeBytes()).split('\n')) {
    if (line.trim().isEmpty) continue;
    final fields = line.trim().split(RegExp(r'\s+'));
    if (fields.length != 2) return null;
    final child = int.tryParse(fields[0]);
    final parent = int.tryParse(fields[1]);
    if (child == null || child <= 0 || parent == null || parent < 0) {
      return null;
    }
    (parents[parent] ??= []).add(child);
  }
  return parents;
}

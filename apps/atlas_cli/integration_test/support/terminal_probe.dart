import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';

/// Exercises the native CLI in a POSIX PTY and prints measured state as JSON.
Future<void> main(List<String> args) async {
  final [binary, home, action] = args;
  final config = File('$home/.atlas/config.yaml');
  await config.parent.create(recursive: true);
  await config.writeAsString('''default_model: test/model
providers:
  - name: test
    type: responses
    base_url: https://example.invalid
    api_key: unused-test-key
    models:
      - value: model
''');
  final terminal = _Terminal();
  Process? process;
  var exited = false;
  try {
    final before = await terminal.state();
    process = await terminal.start(binary, home, action);
    final completion = process.exitCode.then((code) {
      exited = true;
      return code;
    });
    final output = <int>[];
    var sent = false;
    final deadline = Stopwatch()..start();
    while (!exited && deadline.elapsed < const Duration(seconds: 15)) {
      output.addAll(terminal.read());
      final text = utf8.decode(output, allowMalformed: true);
      if (!sent && text.contains('Message Atlas')) {
        if (action == 'quit') {
          terminal.write('/quit\r');
          // First Enter accepts slash completion, second submits it.
          await Future<void>.delayed(const Duration(milliseconds: 200));
          terminal.write('\r');
        } else if (action == 'sigint' || action == 'sigterm') {
          if (!process.kill(
            action == 'sigint' ? ProcessSignal.sigint : ProcessSignal.sigterm,
          )) {
            throw StateError('Could not signal Atlas');
          }
        }
        sent = true;
      }
      await Future<void>.delayed(const Duration(milliseconds: 10));
    }
    final timedOut = !exited;
    if (timedOut) process.kill(ProcessSignal.sigkill);
    final code = await completion.timeout(const Duration(seconds: 5));
    output.addAll(terminal.read());
    final after = await terminal.state();
    final text = utf8.decode(output, allowMalformed: true);
    stdout.writeln(
      jsonEncode({
        'exitCode': code,
        'timedOut': timedOut,
        'restored': before == after,
        'before': before,
        'after': after,
        'rendered': text.contains('Message Atlas'),
        'cursorRestored': text.contains('\x1b[?25h'),
        'alternateScreenLeft': text.contains('\x1b[?1049l'),
        'hasEscapes': text.contains('\x1b'),
      }),
    );
  } finally {
    if (process != null && !exited) {
      process.kill(ProcessSignal.sigkill);
      await process.exitCode.timeout(const Duration(seconds: 5));
    }
    terminal.close();
  }
}

// Only fixed-layout POSIX descriptors are bound here. stty reads termios so
// this probe does not duplicate macOS/Linux-specific terminal flag layouts.
final class _WindowSize extends Struct {
  @Uint16()
  external int rows;
  @Uint16()
  external int columns;
  @Uint16()
  external int xPixels;
  @Uint16()
  external int yPixels;
}

final class _PollFd extends Struct {
  @Int32()
  external int fd;
  @Int16()
  external int events;
  @Int16()
  external int revents;
}

final _libc = DynamicLibrary.process();
final _libutil = Platform.isLinux ? DynamicLibrary.open('libutil.so.1') : _libc;
final _openpty = _libutil
    .lookupFunction<
      Int32 Function(
        Pointer<Int32>,
        Pointer<Int32>,
        Pointer<Utf8>,
        Pointer<Void>,
        Pointer<_WindowSize>,
      ),
      int Function(
        Pointer<Int32>,
        Pointer<Int32>,
        Pointer<Utf8>,
        Pointer<Void>,
        Pointer<_WindowSize>,
      )
    >('openpty');
final _dup = _libc.lookupFunction<Int32 Function(Int32), int Function(int)>(
  'dup',
);
final _dup2 = _libc
    .lookupFunction<Int32 Function(Int32, Int32), int Function(int, int)>(
      'dup2',
    );
final _close = _libc.lookupFunction<Int32 Function(Int32), int Function(int)>(
  'close',
);
// nfds_t is unsigned int on macOS and unsigned long on Linux.
final _poll = Platform.isMacOS
    ? _libc.lookupFunction<
        Int32 Function(Pointer<_PollFd>, Uint32, Int32),
        int Function(Pointer<_PollFd>, int, int)
      >('poll')
    : _libc.lookupFunction<
        Int32 Function(Pointer<_PollFd>, UnsignedLong, Int32),
        int Function(Pointer<_PollFd>, int, int)
      >('poll');
final _read = _libc
    .lookupFunction<
      IntPtr Function(Int32, Pointer<Uint8>, IntPtr),
      int Function(int, Pointer<Uint8>, int)
    >('read');
final _write = _libc
    .lookupFunction<
      IntPtr Function(Int32, Pointer<Uint8>, IntPtr),
      int Function(int, Pointer<Uint8>, int)
    >('write');

class _Terminal() {
  this {
    using((arena) {
      final master = arena<Int32>();
      final slave = arena<Int32>();
      final name = arena<Uint8>(256).cast<Utf8>();
      final size = arena<_WindowSize>();
      size.ref
        ..rows = 32
        ..columns = 100;
      if (_openpty(master, slave, name, nullptr, size) != 0) {
        throw StateError('openpty failed');
      }
      _master = master.value;
      _slave = slave.value;
      _name = name.toDartString();
    });
  }

  late final int _master;
  late final int _slave;
  late final String _name;

  Future<Process> start(String binary, String home, String action) async {
    final input = _dup(0);
    final output = _dup(1);
    try {
      if (input < 0 ||
          output < 0 ||
          _dup2(_slave, 0) < 0 ||
          _dup2(_slave, 1) < 0) {
        throw StateError('Could not attach the PTY to child stdio');
      }
      final environment = Map<String, String>.of(Platform.environment)
        ..remove('NO_COLOR')
        ..['HOME'] = home
        ..['TERM'] = action == 'dumb' ? 'dumb' : 'xterm-256color';
      if (action == 'no_color') environment['NO_COLOR'] = '';
      // Only this short-lived probe's descriptors are changed. The Dart test
      // runner remains untouched; stderr stays separate from the terminal.
      return await Process.start(
        binary,
        const [],
        workingDirectory: home,
        environment: environment,
        includeParentEnvironment: false,
        mode: ProcessStartMode.inheritStdio,
      );
    } finally {
      if (input >= 0) {
        _dup2(input, 0);
        _close(input);
      }
      if (output >= 0) {
        _dup2(output, 1);
        _close(output);
      }
    }
  }

  List<int> read() => using((arena) {
    final descriptor = arena<_PollFd>();
    descriptor.ref
      ..fd = _master
      ..events = 1; // POLLIN
    final buffer = arena<Uint8>(65536);
    final bytes = <int>[];
    while (_poll(descriptor, 1, 0) > 0 && descriptor.ref.revents & 1 != 0) {
      final count = _read(_master, buffer, 65536);
      if (count <= 0) break;
      bytes.addAll(buffer.asTypedList(count));
    }
    return bytes;
  });

  void write(String text) => using((arena) {
    final bytes = utf8.encode(text);
    final buffer = arena<Uint8>(bytes.length);
    buffer.asTypedList(bytes.length).setAll(0, bytes);
    if (_write(_master, buffer, bytes.length) != bytes.length) {
      throw StateError('Could not write terminal input');
    }
  });

  Future<String> state() async {
    final result = await Process.run('stty', [
      Platform.isMacOS ? '-f' : '-F',
      _name,
      '-g',
    ]);
    final state = (result.stdout as String).trim();
    if (result.exitCode != 0 || state.isEmpty) {
      throw StateError('Cannot read terminal state: ${result.stderr}');
    }
    return state;
  }

  void close() {
    _close(_master);
    _close(_slave);
  }
}

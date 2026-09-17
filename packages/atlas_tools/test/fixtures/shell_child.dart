import 'dart:async';
import 'dart:convert';
import 'dart:io';

/// Finite subprocess used to exercise real pipes on every desktop platform.
Future<void> main(List<String> args) async {
  final watchdog = Timer(const Duration(seconds: 8), () => exit(99));
  try {
    switch (args.first) {
      case 'echo':
        await stdin.pipe(stdout);
      case 'invalid':
        stdout.add([0xff, 0x0a]);
        await stdout.flush();
        await Future<void>.delayed(const Duration(milliseconds: 200));
      case 'unicode':
        for (final byte in utf8.encode('你好🙂')) {
          stdout.add([byte]);
          await stdout.flush();
        }
      case 'output':
        stdout.write('first\n');
        await stdout.flush();
        await Future<void>.delayed(const Duration(milliseconds: 300));
        stderr.write('second\n');
        await stderr.flush();
      case 'flood':
        for (var i = 0; i < 100; i++) {
          stdout.write('头${'🙂' * 1000}尾');
          stderr.write('err${'x' * 4000}end');
          await Future.wait([stdout.flush(), stderr.flush()]);
        }
      case 'wait':
        stdout.writeln('ready');
        await stdout.flush();
        await Future<void>.delayed(const Duration(seconds: 5));
      case 'closed_input':
        stdout.writeln('done without reading input');
      case 'controls':
        for (final text in [
          'before\x1b[3',
          '1mred\x1b[0m',
          '\x1b]52;c;hidden',
          '\x1b\\after\r\n',
        ]) {
          stdout.write(text);
          await stdout.flush();
        }
      case 'ignore_term':
        final subscription = ProcessSignal.sigterm.watch().listen((_) {});
        stdout.writeln('pid:$pid');
        await stdout.flush();
        await Future<void>.delayed(const Duration(seconds: 6));
        await subscription.cancel();
      case 'orphan':
        final process = await Process.start(Platform.resolvedExecutable, [
          Platform.script.toFilePath(),
          'wait',
        ], mode: ProcessStartMode.inheritStdio);
        stdout.writeln('child:${process.pid}');
        await stdout.flush();
        exit(0);
      case 'tree':
        final process = await Process.start(Platform.resolvedExecutable, [
          Platform.script.toFilePath(),
          'wait',
        ], mode: ProcessStartMode.inheritStdio);
        await process.exitCode;
      case 'numbers':
        for (var i = 1; i <= 100000; i++) {
          stdout.writeln(i);
        }
        await stdout.flush();
      case 'cwd':
        stdout.write(Directory.current.path);
      case 'exit':
        exitCode = 3;
    }
  } finally {
    watchdog.cancel();
  }
}

@Timeout(Duration(minutes: 2))
library;

import 'dart:convert';
import 'dart:io';

import 'package:atlas_cli/src/version.dart';
import 'package:cli_util/cli_util.dart' as cli_util;
import 'package:io/io.dart' show ExitCode;
import 'package:test/test.dart';
import 'package:test_descriptor/test_descriptor.dart' as d;
import 'package:test_process/test_process.dart';

void main() {
  late String binary;
  final workspace = Directory('../..').absolute.path;

  setUpAll(() async {
    final supplied = Platform.environment['ATLAS_TEST_BINARY'];
    if (supplied != null) {
      binary = File(supplied).absolute.path;
    } else {
      // Code assets require dart build cli, not dart compile exe. Keep this
      // transient bundle out of the release build and the source tree.
      final output = '$workspace/.dart_tool/atlas_cli/integration';
      final dart =
          '${cli_util.sdkPath}/bin/dart${Platform.isWindows ? '.exe' : ''}';
      final build = await TestProcess.start(dart, [
        'build',
        'cli',
        '-t',
        'apps/atlas_cli/bin/atlas.dart',
        '-o',
        output,
      ], workingDirectory: workspace);
      await build.shouldExit(0);
      binary = '$output/bundle/bin/atlas${Platform.isWindows ? '.exe' : ''}';
    }
    expect(File(binary).existsSync(), isTrue);
  });

  Future<TestProcess> start(
    List<String> args, {
    bool configured = false,
  }) async {
    if (configured) {
      await d.dir('.atlas', [
        d.file('config.yaml', '''
default_model: test/model
providers:
  - name: test
    type: responses
    base_url: https://example.invalid
    api_key: unused-test-key
    models:
      - value: model
session:
  db_path: ~/.atlas/test.db
'''),
      ]).create();
    }
    return TestProcess.start(
      binary,
      args,
      workingDirectory: d.sandbox,
      environment: {
        'HOME': d.sandbox,
        'USERPROFILE': d.sandbox,
        'NO_COLOR': '1',
        'ACP_DEBUG': '0',
      },
    );
  }

  for (final args in [
    ['--help'],
    ['-h'],
    ['help'],
    ['acp', '--help'],
    ['server', '--help'],
    ['cache', '--help'],
  ]) {
    test('$args succeeds without configuration', () async {
      final process = await start(args);
      await process.shouldExit(ExitCode.success.code);
      final output = await process.stdout.rest.toList();
      expect(output.join('\n'), contains('Usage: atlas'));
      expect(output.join('\n'), isNot(contains('\x1b')));
      expect(await process.stderr.rest.toList(), isEmpty);
      expect(Directory(d.path('.atlas')).existsSync(), isFalse);
    });
  }

  test('version matches the manifest-generated constant', () async {
    final process = await start(['--version']);
    await process.shouldExit(0);
    expect(await process.stdout.rest.toList(), [packageVersion]);
    expect(await process.stderr.rest.toList(), isEmpty);
  });

  for (final args in [
    ['--unknown'],
    ['acp', '--bad'],
    ['acp', 'extra'],
    ['cache', '--limit', 'bad'],
    ['cache', '--limit'],
    ['server', '--listen=127.0.0.1:0'],
    ['server', '--token-file'],
  ]) {
    test('$args fails before accessing config or data', () async {
      final process = await start(args);
      await process.shouldExit(ExitCode.usage.code);
      expect(await process.stdout.rest.toList(), isEmpty);
      expect(
        (await process.stderr.rest.toList()).join('\n'),
        contains('Usage: atlas'),
      );
      expect(Directory(d.path('.atlas')).existsSync(), isFalse);
    });
  }

  test('cache errors stay off stdout even with valid config', () async {
    final process = await start(['cache', '--limit=bad'], configured: true);
    await process.shouldExit(ExitCode.usage.code);
    expect(await process.stdout.rest.toList(), isEmpty);
    expect(
      (await process.stderr.rest.toList()).join('\n'),
      contains('--limit must be a positive number'),
    );
    expect(File(d.path('.atlas/test.db')).existsSync(), isFalse);
  });

  test('missing configuration uses EX_CONFIG', () async {
    final process = await start(['cache']);
    await process.shouldExit(ExitCode.config.code);
    expect(await process.stdout.rest.toList(), isEmpty);
    expect(
      (await process.stderr.rest.toList()).join('\n'),
      contains('cannot read'),
    );
  });

  test('cache flushes output and closes its database', () async {
    final process = await start(['cache', '--limit=5'], configured: true);
    await process.shouldExit(0);
    expect(
      (await process.stdout.rest.toList()).join('\n'),
      contains('No turns recorded yet (limit 5).'),
    );
    expect(await process.stderr.rest.toList(), isEmpty);
    final db = File(d.path('.atlas/test.db'));
    expect(db.existsSync(), isTrue);
    await db.rename(d.path('.atlas/closed.db'));
  });

  test(
    'ACP flushes responses and exits naturally on EOF after storage use',
    () async {
      final process = await start(['acp'], configured: true);
      process.stdin.writeln(
        jsonEncode({
          'jsonrpc': '2.0',
          'id': 1,
          'method': 'initialize',
          'params': {'protocolVersion': 1},
        }),
      );
      final initialized = jsonDecode(await process.stdout.next) as Map;
      expect(initialized['id'], 1);
      expect(initialized['result'], isNotNull);
      process.stdin.writeln(
        jsonEncode({
          'jsonrpc': '2.0',
          'id': 2,
          'method': 'session/new',
          'params': {'cwd': d.sandbox, 'mcpServers': <Object>[]},
        }),
      );
      final created = jsonDecode(await process.stdout.next) as Map;
      expect(created['id'], 2);
      expect(created['result'], contains('sessionId'));
      await process.stdin.close();
      await process.shouldExit(0);
      final notifications = (await process.stdout.rest.toList())
          .map((line) => jsonDecode(line) as Map)
          .toList();
      expect(notifications, isNotEmpty);
      expect(
        notifications.every((message) => message['method'] == 'session/update'),
        isTrue,
      );
      expect(await process.stderr.rest.toList(), isEmpty);
    },
  );

  test('token rotation needs no provider configuration', () async {
    final process = await start(['server', '--rotate-token']);
    await process.shouldExit(0);
    expect(
      (await process.stdout.rest.toList()).single,
      startsWith('Rotated the remote access token: '),
    );
    expect(
      (await process.stderr.rest.toList()).single,
      contains('Keep it secret'),
    );
    expect(File(d.path('.atlas/remote_token')).existsSync(), isTrue);
    expect(File(d.path('.atlas/test.db')).existsSync(), isFalse);
  });

  test('piped TUI with NO_COLOR emits no terminal escapes', () async {
    final process = await start([]);
    await process.shouldExit(ExitCode.usage.code);
    expect(await process.stdout.rest.toList(), isEmpty);
    final errors = (await process.stderr.rest.toList()).join('\n');
    expect(errors, contains('NO_COLOR'));
    expect(errors, isNot(contains('\x1b')));
  });

  for (final signal in [ProcessSignal.sigint, ProcessSignal.sigterm]) {
    test(
      'ACP cleans up on $signal with stdin still open',
      () async {
        final process = await start(['acp'], configured: true);
        process.stdin.writeln(
          jsonEncode({
            'jsonrpc': '2.0',
            'id': 1,
            'method': 'initialize',
            'params': {'protocolVersion': 1},
          }),
        );
        expect(jsonDecode(await process.stdout.next), contains('result'));
        expect(Process.killPid(process.pid, signal), isTrue);
        await process.shouldExit(0);
        expect(await process.stderr.rest.toList(), isEmpty);
      },
      skip: Platform.isWindows
          ? 'Windows cannot send POSIX signals to children'
          : false,
    );

    test(
      'server releases its listener on $signal',
      () async {
        final probe = await ServerSocket.bind(InternetAddress.loopbackIPv4, 0);
        final port = probe.port;
        await probe.close();
        final process = await start([
          'server',
          '--listen=127.0.0.1:$port',
        ], configured: true);
        await expectLater(
          process.stdout,
          emitsThrough(startsWith('Atlas WebSocket server listening')),
        );
        expect(Process.killPid(process.pid, signal), isTrue);
        await process.shouldExit(0);
        final rebound = await ServerSocket.bind(
          InternetAddress.loopbackIPv4,
          port,
        );
        await rebound.close();
        expect(
          (await process.stderr.rest.toList()).join('\n'),
          isNot(contains('Unhandled')),
        );
      },
      skip: Platform.isWindows
          ? 'Windows cannot send POSIX signals to children'
          : false,
    );
  }

  for (final action in ['quit', 'sigint', 'sigterm', 'no_color', 'dumb']) {
    test(
      'real PTY: $action restores terminal modes and exits',
      () async {
        final probe = await TestProcess.start('${cli_util.sdkPath}/bin/dart', [
          File('integration_test/support/terminal_probe.dart').absolute.path,
          binary,
          d.sandbox,
          action,
        ]);
        await probe.shouldExit(0);
        final lines = await probe.stdout.rest.toList();
        expect(lines, hasLength(1));
        final result = jsonDecode(lines.single) as Map<String, Object?>;
        final errors = (await probe.stderr.rest.toList()).join('\n');
        expect(result['timedOut'], isFalse);
        expect(
          result['restored'],
          isTrue,
          reason: '${result['before']} -> ${result['after']}',
        );
        if (action == 'no_color' || action == 'dumb') {
          expect(result['exitCode'], ExitCode.usage.code);
          expect(result['hasEscapes'], isFalse);
          expect(errors, contains('requires interactive'));
          expect(errors, isNot(contains('\x1b')));
        } else {
          expect(result['exitCode'], 0);
          expect(result['rendered'], isTrue);
          expect(result['cursorRestored'], isTrue);
          expect(result['alternateScreenLeft'], isTrue);
          expect(errors, isEmpty);
        }
      },
      skip: Platform.isWindows
          ? 'POSIX PTY probe; Windows startup tested separately'
          : false,
    );
  }

  test('server startup does not use unsupported platform signals', () async {
    final probe = await ServerSocket.bind(InternetAddress.loopbackIPv4, 0);
    final port = probe.port;
    await probe.close();
    final process = await start([
      'server',
      '--listen=127.0.0.1:$port',
    ], configured: true);
    await expectLater(
      process.stdout,
      emitsThrough(startsWith('Atlas WebSocket server listening')),
    );
    // Exercise an actual connection after the startup message, not just stdout.
    final socket = await Socket.connect(InternetAddress.loopbackIPv4, port);
    socket.destroy();
    await process.kill();
  });
}

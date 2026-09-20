import 'dart:io';

import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_cli/src/version.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:io/io.dart' show ExitCode;
import 'package:test/test.dart';

void main() {
  late StringBuffer out;
  late StringBuffer err;
  late AtlasCommandRunner runner;
  late int loads;

  setUp(() {
    out = StringBuffer();
    err = StringBuffer();
    loads = 0;
    runner = AtlasCommandRunner(
      out: out,
      err: err,
      terminalAvailable: () => false,
      configLoader: () {
        loads++;
        throw const ConfigLoadException('test configuration unavailable');
      },
    );
  });

  group('help and version', () {
    for (final args in [
      ['--help'],
      ['-h'],
      ['help'],
      for (final command in ['acp', 'server', 'cache']) ...[
        [command, '--help'],
        [command, '-h'],
        ['help', command],
      ],
    ]) {
      test('$args does not load configuration', () async {
        expect(await runCli(args, runner: runner), ExitCode.success.code);
        expect(out.toString(), contains('Usage: atlas'));
        expect(err.toString(), isEmpty);
        expect(loads, 0);
      });
    }

    for (final flag in ['--version', '-V']) {
      test('$flag uses the generated package version', () async {
        expect(await runCli([flag], runner: runner), 0);
        expect(out.toString(), '$packageVersion\n');
        expect(err.toString(), isEmpty);
        expect(loads, 0);
      });
    }

    test('generated version matches pubspec', () {
      final manifest = File('pubspec.yaml').readAsStringSync();
      final version = RegExp(
        r'^version:\s*(\S+)',
        multiLine: true,
      ).firstMatch(manifest)?.group(1);
      expect(version, isNotNull);
      expect(
        packageVersion,
        version,
        reason: 'Run mise run cli-version after changing the version',
      );
    });
  });

  group('usage errors', () {
    for (final args in [
      ['unknown'],
      ['--unknown'],
      ['--version', 'cache'],
      ['acp', 'extra'],
      ['acp', '--unknown'],
      ['cache', '--unknown'],
      ['cache', '--limit'],
      ['cache', '--limit', 'bad'],
      ['cache', '--limit=0'],
      ['cache', 'extra'],
      ['server', '--listen'],
      ['server', '--listen=127.0.0.1:0'],
      ['server', '--token-file'],
      ['server', '--token-file='],
      ['server', 'extra'],
    ]) {
      test('$args writes only stderr before configuration', () async {
        expect(await runCli(args, runner: runner), ExitCode.usage.code);
        expect(out.toString(), isEmpty);
        expect(err.toString(), contains('Usage: atlas'));
        expect(loads, 0);
      });
    }
  });

  test('non-interactive TUI is rejected without ANSI output', () async {
    expect(await runCli([], runner: runner), ExitCode.usage.code);
    expect(out.toString(), isEmpty);
    expect(err.toString(), contains('NO_COLOR'));
    expect(err.toString(), isNot(contains('\x1b')));
    expect(loads, 0);
  });

  test('configuration failures use EX_CONFIG', () async {
    expect(await runCli(['cache'], runner: runner), ExitCode.config.code);
    expect(out.toString(), isEmpty);
    expect(err.toString(), contains('test configuration unavailable'));
    expect(loads, 1);
  });

  for (final verbose in [false, true]) {
    test('unexpected failure uses EX_SOFTWARE (verbose=$verbose)', () async {
      runner = AtlasCommandRunner(
        out: out,
        err: err,
        configLoader: () => throw StateError('test failure'),
      );
      expect(
        await runCli([if (verbose) '--verbose', 'cache'], runner: runner),
        ExitCode.software.code,
      );
      expect(out.toString(), isEmpty);
      expect(err.toString(), contains('test failure'));
      expect(err.toString().contains('cli_test.dart'), verbose);
    });
  }
}

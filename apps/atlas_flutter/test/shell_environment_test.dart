import 'dart:io';

import 'package:atlas_flutter/app/shell_environment.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('other platforms preserve the inherited environment', () async {
    final result = await resolveShellEnvironment(
      environment: {'PATH': 'original'},
    );
    expect(result.environment, {'PATH': 'original'});
    expect(result.failure, isNull);
  }, skip: Platform.isMacOS);

  group('macOS shell environment', () {
    late Directory home;
    late Map<String, String> inherited;

    setUp(() async {
      home = await Directory.systemTemp.createTemp('atlas shell env ');
      inherited = {
        'HOME': home.path,
        'ZDOTDIR': home.path,
        'SHELL': '/bin/zsh',
        'PATH': '/usr/bin:/bin:/usr/sbin:/sbin',
        'PWD': '/original/directory',
        'SHLVL': '2',
        '_': '/original/program',
        'ATLAS_INHERITED_TEST': 'kept',
      };
    });
    tearDown(() => home.delete(recursive: true));

    test('loads login and interactive exports without startup noise', () async {
      await File('${home.path}/.zprofile').writeAsString(
        "export ATLAS_LOGIN_TEST=loaded\nprintf 'login banner\\n'\n",
      );
      await File('${home.path}/.zshrc').writeAsString(r'''
export PATH="$HOME/bin:$PATH"
export ATLAS_VALUE_TEST='first=line
second=值'
export ATLAS_EMPTY_TEST=''
export ATLAS_INHERITED_TEST='overridden'
printf 'startup noise\n\377'
printf 'private diagnostic\n' >&2
printf x >> "$HOME/starts"
''');
      final bin = await Directory('${home.path}/bin').create();
      await File('${bin.path}/atlas-env-interpreter')
          .writeAsString('#!/bin/sh\nprintf "%s" "\$ATLAS_VALUE_TEST"\n');
      await File('${bin.path}/atlas-env-command')
          .writeAsString('#!/usr/bin/env atlas-env-interpreter\n');
      await Process.run('/bin/chmod', [
        '+x',
        '${bin.path}/atlas-env-interpreter',
        '${bin.path}/atlas-env-command',
      ]);
      final result = await resolveShellEnvironment(environment: inherited);
      expect(result.failure, isNull);
      expect(result.environment['ATLAS_LOGIN_TEST'], 'loaded');
      expect(result.environment['ATLAS_VALUE_TEST'], 'first=line\nsecond=值');
      expect(result.environment['ATLAS_EMPTY_TEST'], '');
      expect(result.environment['ATLAS_INHERITED_TEST'], 'overridden');
      expect(result.environment['PATH'], startsWith('${bin.path}:'));
      for (final key in ['PWD', 'SHLVL', '_']) {
        expect(result.environment[key], inherited[key]);
      }
      expect(result.environment.containsKey('OLDPWD'), isFalse);
      expect(
        () => result.environment['PATH'] = 'changed',
        throwsUnsupportedError,
      );

      final tool = ShellTool(environment: result.environment);
      final context = ToolContext(
        sessionId: SessionId('session'),
        turnId: TurnId('turn'),
        workingDirectory: home.path,
      );
      for (var i = 0; i < 2; i++) {
        final output = await tool.execute(context, {
          'command': 'atlas-env-command',
        });
        expect(output.metadata['exit_code'], 0);
        expect(output.content, 'first=line\nsecond=值');
      }
      expect(await File('${home.path}/starts').readAsString(), 'x');
    });

    test('defaults to zsh when SHELL is absent', () async {
      inherited.remove('SHELL');
      await File('${home.path}/.zshrc')
          .writeAsString('export ATLAS_DEFAULT_TEST=yes\n');
      final result = await resolveShellEnvironment(environment: inherited);
      expect(result.failure, isNull);
      expect(result.environment['ATLAS_DEFAULT_TEST'], 'yes');
    });

    for (final shell in ['/bin/bash', '/bin/sh']) {
      test('$shell loads an interactive login profile', () async {
        inherited['SHELL'] = shell;
        final profile = shell.endsWith('/bash') ? '.bash_profile' : '.profile';
        await File('${home.path}/$profile')
            .writeAsString('export ATLAS_PROFILE_TEST=loaded\n');
        final result = await resolveShellEnvironment(environment: inherited);
        expect(result.failure, isNull);
        expect(result.environment['ATLAS_PROFILE_TEST'], 'loaded');
      });
    }

    test('rejects unsupported or relative shells', () async {
      for (final shell in ['zsh', '/bin/fish', '/bin/nu']) {
        inherited['SHELL'] = shell;
        final result = await resolveShellEnvironment(environment: inherited);
        expect(result.failure, ShellEnvironmentFailure.unsupportedShell);
        expect(result.environment, inherited);
      }
    });

    test('missing home or shell falls back without exposing values', () async {
      inherited['SHELL'] = '${home.path}/missing/zsh';
      var result = await resolveShellEnvironment(environment: inherited);
      expect(result.failure, ShellEnvironmentFailure.startFailed);
      expect(result.environment, inherited);
      inherited.remove('HOME');
      result = await resolveShellEnvironment(environment: inherited);
      expect(result.failure, ShellEnvironmentFailure.missingHome);
      expect(result.environment, inherited);
    });

    for (final entry in {
      'exit 7': ShellEnvironmentFailure.nonZeroExit,
      'exit 0': ShellEnvironmentFailure.invalidOutput,
    }.entries) {
      test('startup ${entry.key} discards partial exports', () async {
        await File('${home.path}/.zshrc')
            .writeAsString('export ATLAS_PARTIAL_TEST=secret\n${entry.key}\n');
        final result = await resolveShellEnvironment(environment: inherited);
        expect(result.failure, entry.value);
        expect(result.environment, inherited);
      });
    }

    for (final redirect in ['', ' >&2']) {
      test(
        'bounds output on ${redirect.isEmpty ? 'stdout' : 'stderr'}',
        () async {
          await File(
            '${home.path}/.zshrc',
          ).writeAsString('while true; do printf "%0100d" 0$redirect; done\n');
          final result = await resolveShellEnvironment(
            environment: inherited,
            maxOutputBytes: 1024,
          );
          expect(result.failure, ShellEnvironmentFailure.outputLimit);
          expect(result.environment, inherited);
        },
      );
    }

    test(
      'timeout kills the probe and retains the original environment',
      () async {
        await File('${home.path}/.zshrc').writeAsString(r'''
printf '%s' "$$" > "$HOME/probe.pid"
export ATLAS_PARTIAL_TEST=secret
while true; do :; done
''');
        final clock = Stopwatch()..start();
        final result = await resolveShellEnvironment(
          environment: inherited,
          timeout: const Duration(milliseconds: 500),
        );
        expect(result.failure, ShellEnvironmentFailure.timedOut);
        expect(result.environment, inherited);
        expect(clock.elapsed, lessThan(const Duration(seconds: 2)));
        final pid = (await File(
          '${home.path}/probe.pid',
        ).readAsString()).trim();
        await _expectProcessExited(int.parse(pid), 'probe');
      },
    );

    for (final shell in ['/bin/zsh', '/bin/bash', '/bin/sh']) {
      for (final overflow in [false, true]) {
        test(
          '$shell cleans foreground descendants on ${overflow ? 'output overflow' : 'timeout'}',
          () async {
            inherited['SHELL'] = shell;
            final profile = switch (shell) {
              '/bin/zsh' => '.zshrc',
              '/bin/bash' => '.bash_profile',
              _ => '.profile',
            };
            await File('${home.path}/$profile').writeAsString(r'''
printf '%s' "$$" > "$HOME/probe.pid"
/bin/sh "$HOME/foreground.sh"
''');
            await File('${home.path}/foreground.sh').writeAsString(r'''
printf '%s' "$$" > "$HOME/foreground.pid"
trap '' TERM
/bin/sh "$HOME/leaf.sh"
''');
            await File('${home.path}/leaf.sh').writeAsString(
              r'''printf '%s' "$$" > "$HOME/leaf.pid"
trap '' TERM
''' +
                  (overflow
                      ? 'while true; do printf "%0100d" 0; done\n'
                      : 'exec /bin/sleep 60\n'),
            );
            final unrelated = await Process.start('/bin/sleep', ['60']);
            await unrelated.stdin.close();
            addTearDown(() async {
              unrelated.kill(ProcessSignal.sigkill);
              await unrelated.exitCode;
              for (final name in ['probe', 'foreground', 'leaf']) {
                final file = File('${home.path}/$name.pid');
                if (await file.exists()) {
                  Process.killPid(
                    int.parse(await file.readAsString()),
                    ProcessSignal.sigkill,
                  );
                }
              }
            });
            final clock = Stopwatch()..start();
            final result = await resolveShellEnvironment(
              environment: inherited,
              timeout: const Duration(milliseconds: 750),
              maxOutputBytes: 1024,
            );
            expect(
              result.failure,
              overflow
                  ? ShellEnvironmentFailure.outputLimit
                  : ShellEnvironmentFailure.timedOut,
            );
            expect(result.environment, inherited);
            expect(clock.elapsed, lessThan(const Duration(seconds: 3)));
            for (final name in ['probe', 'foreground', 'leaf']) {
              final pid = int.parse(
                await File('${home.path}/$name.pid').readAsString(),
              );
              await _expectProcessExited(pid, name);
            }
            final alive = await Process.run('/bin/kill', [
              '-0',
              '${unrelated.pid}',
            ]);
            expect(
              alive.exitCode,
              0,
              reason: 'Unrelated processes must be preserved',
            );
          },
        );
      }
    }

    test(
      'an already reparented descendant cannot block startup indefinitely',
      () async {
        await File('${home.path}/.zshrc').writeAsString(r'''
/bin/sleep 20 &
printf '%s' "$!" > "$HOME/child.pid"
''');
        addTearDown(() async {
          final file = File('${home.path}/child.pid');
          if (await file.exists()) {
            Process.killPid(
              int.parse(await file.readAsString()),
              ProcessSignal.sigkill,
            );
          }
        });
        final clock = Stopwatch()..start();
        final result = await resolveShellEnvironment(
          environment: inherited,
          timeout: const Duration(milliseconds: 500),
        );
        expect(result.failure, ShellEnvironmentFailure.timedOut);
        expect(result.environment, inherited);
        expect(clock.elapsed, lessThan(const Duration(seconds: 2)));
      },
    );
  }, skip: !Platform.isMacOS);
}

Future<void> _expectProcessExited(int pid, String name) async {
  final clock = Stopwatch()..start();
  while (clock.elapsed < const Duration(seconds: 2)) {
    final running = await Process.run('/bin/kill', ['-0', '$pid']);
    if (running.exitCode != 0) return;
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
  fail('$name process $pid survived shell environment cleanup');
}

import 'dart:io';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:atlas_flutter/app/runtime_environment.dart';

import 'package:flutter_test/flutter_test.dart';

import 'package:atlas_flutter/app/acp_bootstrap.dart';
import 'package:atlas_flutter/app/acp_connections.dart';

void main() {
  test('AcpConnection carries the server command', () {
    const connection = AcpConnection(
      name: 'Atlas',
      command: 'atlas',
      arguments: ['acp'],
    );
    expect(connection.name, 'Atlas');
    expect(connection.command, 'atlas');
    expect(connection.arguments, ['acp']);
  });

  test('ACP resolves commands with the shared startup environment across activations', () async {
    final home = await Directory.systemTemp.createTemp('atlas acp env ');
    addTearDown(() => home.delete(recursive: true));
    final bin = await Directory('${home.path}/bin').create();
    final record = File('${home.path}/record.jsonl');
    final dartLookup = await Process.run('/usr/bin/which', ['dart']);
    expect(dartLookup.exitCode, 0);
    final dart = (dartLookup.stdout as String).trim();
    final fixture = File('test/fixtures/environment_acp_agent.dart')
        .absolute
        .path;
    final command = File('${bin.path}/atlas-env-acp');
    await command.writeAsString(
      '#!/bin/sh\nexec ${_quote(dart)} ${_quote(fixture)} ${_quote(record.path)}\n',
    );
    await Process.run('/bin/chmod', ['+x', command.path]);
    final values = {
      'HOME': home.path,
      'PATH': '${bin.path}:/usr/bin:/bin',
      'ATLAS_ACP_ENV_TEST': 'startup',
    };
    final controller = createRuntimeEnvironmentController(environment: values);
    values['ATLAS_ACP_ENV_TEST'] = 'changed-after-startup';
    final container = ProviderContainer(
      overrides: [runtimeEnvironmentProvider.overrideWith(() => controller)],
    );
    addTearDown(container.dispose);
    container.read(runtimeEnvironmentProvider);
    const connection = AcpConnection(name: 'test', command: 'atlas-env-acp');
    for (var i = 0; i < 2; i++) {
      await controller.activateConnection(connection);
      expect(
        container.read(runtimeEnvironmentProvider).status,
        AcpConnectionStatus.connected,
      );
    }
    await container.read(runtimeEnvironmentProvider).environment!.close();
    final records = (await record.readAsLines()).map(
      (line) => jsonDecode(line),
    );
    expect(records, [
      for (var i = 0; i < 2; i++)
        {'value': 'startup', 'home': home.path, 'cwd': home.path},
    ]);
  }, skip: Platform.isWindows);

  test('bootstrapAcpClient reports a failed process start', () async {
    final missing = Platform.isWindows
        ? r'Z:\nonexistent\atlas-acp'
        : '/nonexistent/atlas-acp';
    final bootstrap = await bootstrapAcpClient(
      AcpConnection(name: 'missing', command: missing),
    );
    expect(bootstrap.environment, isNull);
    expect(bootstrap.error, contains('Cannot start ACP server'));
  });
}

String _quote(String value) => "'${value.replaceAll("'", "'\"'\"'")}'";

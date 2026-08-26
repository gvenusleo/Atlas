import 'dart:io';

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

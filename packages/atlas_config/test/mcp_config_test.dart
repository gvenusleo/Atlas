import 'dart:convert';

import 'package:atlas_config/atlas_config.dart';
import 'package:test/test.dart';

void main() {
  AtlasConfig parse(
    Map<String, Object?> servers, {
    Map<String, String> env = const {},
  }) => parseConfig(
    '{}',
    mcpText: jsonEncode({'mcpServers': servers}),
    environment: env,
    homeDirectory: '/home/test',
  );

  test(
    'maps common Pi transports and substitutes only enabled credentials',
    () {
      final config = parse(
        {
          'local': {
            'command': 'node',
            'args': ['server.js', ''],
            'cwd': '~/project',
            'env': {'TOKEN': r'${TOKEN}'},
          },
          'remote': {
            'type': 'http',
            'url': 'https://example.com/mcp?workspace=one',
            'headers': {'Authorization': r'Bearer ${TOKEN}'},
            'timeout': 120,
          },
          'off': {
            'enabled': false,
            'command': 'missing',
            'env': {'TOKEN': r'${MISSING}'},
          },
        },
        env: {'TOKEN': 'secret'},
      );
      final local = config.mcpServers[0] as McpStdioConfig;
      expect(local.workingDirectory, '/home/test/project');
      expect(local.environment, {'TOKEN': 'secret'});
      expect(local.args, ['server.js', '']);
      expect(local.startupTimeout, const Duration(seconds: 15));
      final remote = config.mcpServers[1] as McpHttpConfig;
      expect(remote.url.query, 'workspace=one');
      expect(remote.headers, {'Authorization': 'Bearer secret'});
      expect(remote.callTimeout, const Duration(seconds: 120));
      expect(config.mcpServers[2].enabled, isFalse);
    },
  );

  test('missing enabled secret reports field without values', () {
    expect(
      () => parse({
        'remote': {
          'url': 'https://example.com/mcp',
          'headers': {'Authorization': r'${MISSING}'},
        },
      }),
      throwsA(
        isA<ConfigLoadException>().having(
          (e) => e.message,
          'message',
          contains('mcp.mcpServers.remote.headers.Authorization'),
        ),
      ),
    );
  });

  test('invalid shapes and transport fields fail with field paths', () {
    for (final entry in <String, Object?>{
      'enabled': 'yes',
      'startupTimeout': 0,
      'timeout': 9223372036855,
      'cwd': 'relative',
      'args': [2],
      'url': 'https://example.com',
      'env': {'TOKEN': 123},
    }.entries) {
      expect(
        () => parse({
          'local': {'command': 'node', entry.key: entry.value},
        }),
        throwsA(
          isA<ConfigLoadException>().having(
            (e) => e.message,
            'field',
            contains('mcp.mcpServers.local.${entry.key}'),
          ),
        ),
      );
    }
    expect(
      () => parse({
        'remote': {'type': 'sse', 'url': 'https://example.com'},
      }),
      throwsA(isA<ConfigLoadException>()),
    );
  });

  test('rejects unsafe or SDK-owned HTTP headers after substitution', () {
    for (final header in [
      'Mcp-Session-Id',
      'Host',
      'Content-Type',
      'Mcp-Param-Tenant',
    ]) {
      expect(
        () => parse({
          'remote': {
            'url': 'https://example.com',
            'headers': {header: 'value'},
          },
        }),
        throwsA(isA<ConfigLoadException>()),
      );
    }
    expect(
      () => parse(
        {
          'remote': {
            'url': 'https://example.com',
            'headers': {'Authorization': r'${TOKEN}'},
          },
        },
        env: {'TOKEN': 'secret\r\nInjected: true'},
      ),
      throwsA(
        isA<ConfigLoadException>().having(
          (e) => e.message,
          'redacted',
          isNot(contains('secret')),
        ),
      ),
    );
  });

  test('omitted configuration stays empty', () {
    expect(parseConfig('{}').mcpServers, isEmpty);
  });
}

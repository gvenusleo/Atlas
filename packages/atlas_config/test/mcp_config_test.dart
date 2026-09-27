import 'package:atlas_config/atlas_config.dart';
import 'package:test/test.dart';

const _base = '''
default_model: test/model
providers:
  - name: test
    type: responses
    base_url: https://example.com
    api_key: unused
    models:
      - value: model
''';

void main() {
  AtlasConfig parse(String servers, {Map<String, String> env = const {}}) =>
      parseConfig(
        '$_base\nmcp_servers:\n$servers',
        environment: env,
        homeDirectory: '/home/test',
      );

  test('maps transports and substitutes only enabled credentials', () {
    final config = parse(
      r'''
  - name: local
    transport: stdio
    command: node
    args: [server.js, ""]
    cwd: ~/project
    env:
      TOKEN: ${TOKEN}
  - name: remote
    transport: streamable_http
    url: https://example.com/mcp?workspace=one
    headers:
      Authorization: Bearer ${TOKEN}
    call_timeout_seconds: 120
  - name: off
    transport: stdio
    enabled: false
    command: missing
    env:
      TOKEN: ${MISSING}
''',
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
  });

  test('missing enabled secret reports field without values', () {
    expect(
      () => parse(r'''
  - name: remote
    transport: streamable_http
    url: https://example.com/mcp
    headers:
      Authorization: ${MISSING}
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (e) => e.message,
          'message',
          contains('mcp_servers[0].headers.Authorization'),
        ),
      ),
    );
  });

  test('invalid shapes and transport fields fail with field paths', () {
    for (final entry in <(String, String)>[
      ('enabled: yes', 'enabled'),
      ('startup_timeout_seconds: 0', 'startup_timeout_seconds'),
      ('call_timeout_seconds: 9223372036855', 'call_timeout_seconds'),
      ('cwd: relative', 'cwd'),
      ('args: [2]', 'args[0]'),
      ('url: https://example.com', 'url'),
      ('env: {TOKEN: 123}', 'env.TOKEN'),
    ]) {
      expect(
        () => parse(
          '  - name: local\n    transport: stdio\n    command: node\n    ${entry.$1}\n',
        ),
        throwsA(
          isA<ConfigLoadException>().having(
            (e) => e.message,
            'field',
            contains('mcp_servers[0].${entry.$2}'),
          ),
        ),
      );
    }
    expect(
      () => parse(
        '  - name: same\n    transport: stdio\n    command: node\n  - name: same\n    transport: stdio\n    command: node\n',
      ),
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
        () => parse(
          '  - name: remote\n    transport: streamable_http\n    url: https://example.com\n    headers:\n      $header: value\n',
        ),
        throwsA(isA<ConfigLoadException>()),
      );
    }
    expect(
      () => parse(
        r'''
  - name: remote
    transport: streamable_http
    url: https://example.com
    headers:
      Authorization: ${TOKEN}
''',
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
    expect(parseConfig(_base).mcpServers, isEmpty);
  });
}

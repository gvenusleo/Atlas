import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'local ACP runtime shares startup exports with config and shell tools',
    () async {
      final home = await Directory.systemTemp.createTemp(
        'atlas bootstrap env ',
      );
      addTearDown(() => home.delete(recursive: true));
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      addTearDown(() => server.close(force: true));
      final authorizations = <String?>[];
      final requests = <Map<String, dynamic>>[];
      final command = Platform.isWindows
          ? r'Write-Output $env:ATLAS_LOCAL_ENV_TEST'
          : r'printf "%s" "$ATLAS_LOCAL_ENV_TEST"';
      unawaited(
        server.forEach((request) async {
          authorizations.add(
            request.headers.value(HttpHeaders.authorizationHeader),
          );
          requests.add(
            jsonDecode(await utf8.decoder.bind(request).join())
                as Map<String, dynamic>,
          );
          request.response.headers.contentType = ContentType(
            'text',
            'event-stream',
          );
          final delta = requests.length == 1
              ? <String, Object?>{
                  'tool_calls': [
                    {
                      'index': 0,
                      'id': 'environment-check',
                      'type': 'function',
                      'function': {
                        'name': 'shell',
                        'arguments': jsonEncode({'command': command}),
                      },
                    },
                  ],
                }
              : <String, Object?>{'content': 'Environment verified.'};
          for (final chunk in [
            {
              'choices': [
                {'delta': delta},
              ],
            },
            {
              'choices': [
                {
                  'delta': <String, Object?>{},
                  'finish_reason': requests.length == 1 ? 'tool_calls' : 'stop',
                },
              ],
            },
          ]) {
            request.response.write('data: ${jsonEncode(chunk)}\n\n');
          }
          request.response.write('data: [DONE]\n\n');
          await request.response.close();
        }),
      );
      final config = File('${home.path}/.atlas/config.yaml');
      await config.parent.create();
      await config.writeAsString('''default_model: local/test
providers:
  - name: local
    type: chat_completions
    base_url: http://127.0.0.1:${server.port}/v1
    api_key: \${ATLAS_BOOTSTRAP_TEST_KEY}
    models:
      - value: test
        context_window: 100000
''');
      final values = {
        ...Platform.environment,
        'HOME': home.path,
        'ATLAS_BOOTSTRAP_TEST_KEY': 'test-only-key',
        'ATLAS_LOCAL_ENV_TEST': 'startup-tool-value',
      };
      final bootstrap = await bootstrapRuntime(environment: values);
      expect(bootstrap.error, isNull);
      final environment = bootstrap.environment!;
      addTearDown(environment.close);
      values['ATLAS_LOCAL_ENV_TEST'] = 'mutated-after-bootstrap';
      await environment.runtime
          .run(
            TurnRequest(
              content: const [TextContent('Check the environment')],
              workingDirectory: home.path,
            ),
          )
          .toList();
      expect(requests, hasLength(2));
      expect(authorizations, everyElement('Bearer test-only-key'));
      final messages = requests.last['messages'] as List<dynamic>;
      final toolMessage = messages.cast<Map<String, dynamic>>().singleWhere(
        (message) => message['role'] == 'tool',
      );
      expect(toolMessage['content'], contains('startup-tool-value'));
      expect(
        toolMessage['content'],
        isNot(contains('mutated-after-bootstrap')),
      );
      expect(File('${home.path}/.atlas/atlas.db').existsSync(), isTrue);
    },
  );
}

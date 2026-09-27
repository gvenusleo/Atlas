import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../../packages/atlas_mcp/test/fixtures/server.dart';

void main() {
  for (final mcp in [false, true]) {
    test(
      'local ACP runtime shares startup exports with ${mcp ? 'MCP' : 'shell'} tools',
      () async {
        final home = await Directory.systemTemp.createTemp(
          'atlas bootstrap env ',
        );
        addTearDown(() => home.delete(recursive: true));
        final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
        addTearDown(() => server.close(force: true));
        final peer = FixturePeer();
        final mcpAuthorizations = <String?>[];
        final authorizations = <String?>[];
        final requests = <Map<String, dynamic>>[];
        final command = Platform.isWindows
            ? r'Write-Output $env:ATLAS_LOCAL_ENV_TEST'
            : r'printf "%s" "$ATLAS_LOCAL_ENV_TEST"';
        unawaited(
          server.forEach((request) async {
            if (request.uri.path == '/mcp') {
              mcpAuthorizations.add(request.headers.value('authorization'));
              if (request.method != 'POST') {
                request.response.statusCode = 405;
                await request.response.close();
                return;
              }
              final message = jsonDecode(
                await utf8.decoder.bind(request).join(),
              ) as Map<String, dynamic>;
              final responses = <Map<String, dynamic>>[];
              await peer.handle(message, responses.add);
              request.response.headers.contentType = ContentType.json;
              request.response.statusCode =
                  message['method'] == 'server/discover'
                  ? 400
                  : responses.isEmpty
                  ? 202
                  : 200;
              if (responses.isNotEmpty) {
                request.response.write(jsonEncode(responses.last));
              }
              await request.response.close();
              return;
            }
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
                          'name': mcp
                              ? ((requests.last['tools'] as List<dynamic>)
                                        .cast<Map<String, dynamic>>()
                                        .firstWhere(
                                          (t) =>
                                              (t['function']
                                                  as Map<
                                                    String,
                                                    dynamic
                                                  >)['description'] ==
                                              'echo',
                                        )['function']
                                    as Map<String, dynamic>)['name']
                              : 'shell',
                          'arguments': jsonEncode(
                            mcp
                                ? {'text': 'startup-tool-value'}
                                : {'command': command},
                          ),
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
                    'finish_reason': requests.length == 1
                        ? 'tool_calls'
                        : 'stop',
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
        if (mcp) {
          await config.writeAsString('''mcp_servers:
  - name: remote
    transport: streamable_http
    url: http://127.0.0.1:${server.port}/mcp
    headers:
      Authorization: Bearer \${ATLAS_BOOTSTRAP_TEST_KEY}
''', mode: FileMode.append);
        }
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
        final events = await environment.runtime
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
        if (mcp) {
          expect(mcpAuthorizations, isNotEmpty);
          expect(mcpAuthorizations, everyElement('Bearer test-only-key'));
          expect(peer.calls, ['echo']);
          final result = events.whereType<ToolFinished>().single.result;
          final restored = await environment.runtime.loadSession(
            result.sessionId,
          );
          expect(
            restored.timeline.whereType<ToolResultItem>().single.content,
            result.content,
          );
          expect(
            restored.timeline.whereType<ToolCallItem>().single.call.name,
            startsWith('mcp_'),
          );
        }
      },
    );
  }
}

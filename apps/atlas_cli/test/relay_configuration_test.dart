import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:test/test.dart';

void main() {
  test(
    'one Pi relay config controls all three protocols and request-time auth',
    () async {
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      addTearDown(() => server.close(force: true));
      final captured =
          <
            ({
              String path,
              String? authorization,
              String? apiKey,
              String? account,
              String? affinity,
              Map<String, dynamic> body,
            })
          >[];
      unawaited(
        server.forEach((request) async {
          final body = jsonDecode(
            await utf8.decoder.bind(request).join(),
          ) as Map<String, dynamic>;
          captured.add((
            path: request.uri.path,
            authorization: request.headers.value('authorization'),
            apiKey: request.headers.value('x-api-key'),
            account: request.headers.value('x-account'),
            affinity: request.headers.value('x-session-id'),
            body: body,
          ));
          request.response.headers.contentType = ContentType(
            'text',
            'event-stream',
          );
          final chunks = switch (request.uri.path) {
            '/v1/chat/completions' => [
              {
                'choices': [
                  {
                    'delta': {'content': 'chat'},
                  },
                ],
              },
            ],
            '/v1/responses' => [
              {'type': 'response.output_text.delta', 'delta': 'responses'},
              {
                'type': 'response.completed',
                'response': {
                  'id': 'r',
                  'status': 'completed',
                  'output': <Object?>[],
                },
              },
            ],
            '/anthropic/v1/messages' => [
              {
                'type': 'message_start',
                'message': {
                  'usage': {'input_tokens': 1},
                },
              },
              {
                'type': 'content_block_delta',
                'index': 0,
                'delta': {'type': 'text_delta', 'text': 'claude'},
              },
              {
                'type': 'message_delta',
                'delta': {'stop_reason': 'end_turn'},
                'usage': {'output_tokens': 1},
              },
              {'type': 'message_stop'},
            ],
            _ => throw StateError('Unexpected request path'),
          };
          for (final chunk in chunks) {
            request.response.write('data: ${jsonEncode(chunk)}\n\n');
          }
          if (request.uri.path == '/v1/chat/completions') {
            request.response.write('data: [DONE]\n\n');
          }
          await request.response.close();
        }),
      );
      final dir = await Directory.systemTemp.createTemp('atlas_relay_');
      addTearDown(() => dir.delete(recursive: true));
      final auth = AuthStore(File('${dir.path}/auth.json'));
      await auth.setKey('relay', 'stored-first');
      final origin = 'http://127.0.0.1:${server.port}';
      final config = parseConfig(
        '{"defaultProvider":"relay","defaultModel":"chat"}',
        catalog: ModelCatalog({}),
        authStore: auth,
        modelsText: jsonEncode({
          'providers': {
            'relay': {
              'baseUrl': '$origin/v1',
              'api': 'openai-completions',
              'apiKey': 'configured-lower-priority',
              'headers': {'X-Account': 'provider'},
              'models': [
                {
                  'id': 'chat',
                  'reasoning': true,
                  'thinkingLevelMap': {'high': 'max'},
                  'headers': {'x-account': 'model'},
                  'compat': {
                    'supportsUsageInStreaming': false,
                    'supportsFinishReason': false,
                    'maxTokensField': 'max_tokens',
                    'sendSessionAffinityHeaders': true,
                    'sessionAffinityFormat': 'openrouter',
                  },
                  'samplingParams': {'top_p': 0.8},
                },
                {
                  'id': 'responses',
                  'api': 'openai-responses',
                  'compat': {
                    'supportsMaxOutputTokens': false,
                    'supportsStrictMode': false,
                    'supportsDeveloperRole': false,
                  },
                },
                {
                  'id': 'claude',
                  'api': 'anthropic-messages',
                  'baseUrl': '$origin/anthropic/v1',
                  'reasoning': true,
                  'thinkingLevelMap': {'high': 'high'},
                  'compat': {
                    'forceAdaptiveThinking': true,
                    'supportsCacheControlOnTools': false,
                    'supportsTemperature': false,
                  },
                },
              ],
            },
          },
        }),
      );
      final store = DriftSessionStore.inMemory();
      addTearDown(store.close);
      final client = DioHttpStreamClient();
      addTearDown(client.close);
      final runtime = composeRuntime(config, store: store, httpClient: client);
      addTearDown(runtime.shutdown);
      for (final id in ['chat', 'responses', 'claude']) {
        if (id == 'responses') await auth.setKey('relay', 'stored-rotated');
        final events = await runtime.provider
            .stream(
              ModelRequest(
                sessionId: SessionId('session'),
                turnId: TurnId(id),
                model: ModelRef(
                  providerId: ProviderId('relay'),
                  modelId: ModelId(id),
                ),
                messages: const [
                  ModelMessage(
                    role: ModelMessageRole.user,
                    content: [TextContent('hello')],
                  ),
                ],
                systemPrompt: 'system',
                reasoningEffort: id == 'responses' ? null : 'high',
                maxOutputTokens: 2048,
                tools: const [
                  ToolDescriptor(
                    name: 'lookup',
                    description: 'lookup',
                    inputSchema: {'type': 'object'},
                  ),
                ],
              ),
            )
            .toList();
        expect(
          events.whereType<ModelFailedEvent>(),
          isEmpty,
          reason: '$id must complete',
        );
        expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
      }
      expect(captured.map((r) => r.path), [
        '/v1/chat/completions',
        '/v1/responses',
        '/anthropic/v1/messages',
      ]);
      final chat = captured[0];
      expect(chat.authorization, 'Bearer stored-first');
      expect(chat.account, 'model');
      expect(chat.affinity, 'session');
      expect(chat.body['max_tokens'], 2048);
      expect(chat.body['reasoning_effort'], 'max');
      expect(chat.body['top_p'], 0.8);
      expect(chat.body, isNot(contains('max_completion_tokens')));
      expect(chat.body, isNot(contains('stream_options')));
      expect(chat.body, isNot(contains('prompt_cache_key')));
      final responses = captured[1];
      expect(responses.authorization, 'Bearer stored-rotated');
      expect(responses.body, isNot(contains('max_output_tokens')));
      expect(responses.body, isNot(contains('instructions')));
      expect((responses.body['input'] as List).first['role'], 'system');
      expect(
        (responses.body['tools'] as List).single,
        isNot(contains('strict')),
      );
      final claude = captured[2];
      expect(claude.apiKey, 'stored-rotated');
      expect(claude.body['thinking'], {'type': 'adaptive'});
      expect(claude.body['output_config'], {'effort': 'high'});
      expect(
        (claude.body['tools'] as List).single,
        isNot(contains('cache_control')),
      );
    },
  );
}

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  test('streams Anthropic text, usage, and stop reason', () async {
    final requests = <Map<String, Object?>>[];
    final server = await _startServer((request) async {
      requests.add(
        jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, Object?>,
      );
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":5,"cache_creation_input_tokens":2,"cache_read_input_tokens":1,"output_tokens":0}}}',
        '{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}',
        '{"type":"content_block_stop","index":0}',
        '{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    final provider = _provider(server);
    final events = await provider.stream(_request()).toList();

    expect(events.whereType<TextDeltaEvent>().single.delta, 'Hello');
    final response = (events.last as ModelCompletedEvent).response;
    expect(response.stopReason, StopReason.endTurn);
    expect(response.usage.totalTokens, 8);
    expect(response.usage.cacheReadInputTokens, 1);
    expect(requests.single['stream'], isTrue);
    expect(requests.single['max_tokens'], 4096);
    expect(requests.single['system'], 'Be concise.');
  });

  test('streams Anthropic tool use from split input JSON', () async {
    final server = await _startServer((request) async {
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":4,"output_tokens":0}}}',
        '{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool-1","name":"lookup","input":{}}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\\"q\\":"}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\\"x\\",\\"nested\\":{\\"a\\":[1,2]}}"}}',
        '{"type":"content_block_stop","index":0}',
        '{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":2}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    final provider = _provider(server);
    final events = await provider.stream(_request()).toList();

    final response = (events.last as ModelCompletedEvent).response;
    expect(response.stopReason, StopReason.toolUse);
    final call = response.toolCalls.single;
    expect(call.name, 'lookup');
    expect(call.arguments['q'], 'x');
    expect((call.arguments['nested'] as Map<String, Object?>)['a'], [1, 2]);
  });

  test('accepts Anthropic tool use with empty input', () async {
    final server = await _startServer((request) async {
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":0}}}',
        '{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool-1","name":"noop","input":{}}}',
        '{"type":"content_block_stop","index":0}',
        '{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);
    final events = await _provider(server).stream(_request()).toList();
    final call = (events.last as ModelCompletedEvent).response.toolCalls.single;
    expect(call.arguments, isEmpty);
  });

  test('surfaces HTTP errors as provider exceptions', () async {
    final server = await _startServer((request) async {
      request.response.statusCode = HttpStatus.badRequest;
      request.response.write('{"error":{"message":"secret user prompt"}}');
      await request.response.close();
    });
    addTearDown(server.close);

    final events = await _provider(server).stream(_request()).toList();
    final error = (events.single as ModelFailedEvent).error;
    expect(error, isA<AnthropicProviderException>());
    final providerError = error as AnthropicProviderException;
    expect(providerError.statusCode, 400);
    expect(providerError.message, 'provider request failed (status 400)');
    expect(providerError.message, isNot(contains('secret user prompt')));
  });

  test('does not expose an Anthropic stream error message', () async {
    final server = await _startServer((request) async {
      await _sendSse(request.response, [
        '{"type":"error","error":{"type":"api_error","message":"secret user prompt"}}',
      ]);
    });
    addTearDown(server.close);

    final events = await _provider(server).stream(_request()).toList();
    final error = (events.single as ModelFailedEvent).error;
    expect(error, isA<AnthropicProviderException>());
    expect(
      (error as AnthropicProviderException).message,
      'anthropic stream failed',
    );
    expect(error.message, isNot(contains('secret user prompt')));
  });

  test('captures thinking blocks with signatures for replay', () async {
    final server = await _startServer((request) async {
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":0}}}',
        '{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me think"}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" more"}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}',
        '{"type":"content_block_stop","index":0}',
        '{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"opaque-data"}}',
        '{"type":"content_block_stop","index":1}',
        '{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}',
        '{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Done"}}',
        '{"type":"content_block_stop","index":2}',
        '{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    final events = await _provider(server).stream(_request()).toList();
    expect(
      events.whereType<ReasoningDeltaEvent>().map((event) => event.delta),
      ['let me think', ' more'],
    );
    final response = (events.last as ModelCompletedEvent).response;
    expect(response.reasoning, 'let me think more');
    expect(response.continuation?.reasoningSummary, 'let me think more');
    final blocks =
        (response.continuation!.opaquePayload['thinking_blocks'] as List)
            .cast<Map<String, Object?>>();
    expect(blocks, hasLength(2));
    expect(blocks.first['type'], 'thinking');
    expect(blocks.first['signature'], 'sig-1');
    expect(blocks.first['thinking'], 'let me think more');
    expect(blocks.last, {'type': 'redacted_thinking', 'data': 'opaque-data'});
  });

  test('replays empty and redacted thinking blocks on the next request', () async {
    final requests = <Map<String, Object?>>[];
    final server = await _startServer((request) async {
      requests.add(
        jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, Object?>,
      );
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":0}}}',
        '{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    final provider = _provider(server);
    await provider
        .stream(
          _request(
            messages: [
              ModelMessage(
                role: ModelMessageRole.assistant,
                content: const [TextContent('Done')],
                continuation: ModelContinuation(
                  providerId: _modelRef.providerId,
                  reasoningSummary: 'thought',
                  opaquePayload: const <String, Object?>{
                    'thinking_blocks': [
                      {
                        'type': 'thinking',
                        'thinking': '',
                        'signature': 'sig-1',
                      },
                      {'type': 'redacted_thinking', 'data': 'opaque-data'},
                    ],
                  },
                ),
              ),
              const ModelMessage(
                role: ModelMessageRole.user,
                content: [TextContent('again')],
              ),
            ],
          ),
        )
        .toList();

    final messages = (requests.single['messages'] as List)
        .cast<Map<String, Object?>>();
    final content = (messages.first['content'] as List)
        .cast<Map<String, Object?>>();
    expect(content.first['type'], 'thinking');
    expect(content.first['signature'], 'sig-1');
    expect(content.first['thinking'], '');
    expect(content[1], {'type': 'redacted_thinking', 'data': 'opaque-data'});
  });

  test('sends Anthropic headers, tools, and thinking configuration', () async {
    final requests = <Map<String, Object?>>[];
    final headers = <String, String>{};
    final server = await _startServer((request) async {
      request.headers.forEach((name, values) {
        headers[name] = values.first;
      });
      requests.add(
        jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, Object?>,
      );
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":0}}}',
        '{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    final provider = _provider(
      server,
      thinkingBudgetTokens: 2048,
      maxOutputTokens: 4096,
    );
    await provider.stream(_request(temperature: 0.7)).toList();

    expect(headers['x-api-key'], 'secret-key');
    expect(headers['anthropic-version'], '2023-06-01');
    final body = requests.single;
    expect(body['max_tokens'], 4096);
    expect(body['thinking'], {'type': 'enabled', 'budget_tokens': 2048});
    expect(body.containsKey('temperature'), isFalse);
    final tools = (body['tools'] as List).cast<Map<String, Object?>>();
    expect(tools.single['name'], 'lookup');
    expect(tools.single['input_schema'], {'type': 'object'});
  });

  test('rejects a thinking budget that reaches max_tokens', () async {
    var requests = 0;
    final server = await _startServer((request) async {
      requests++;
      await request.response.close();
    });
    addTearDown(server.close);

    final events = await _provider(
      server,
      thinkingBudgetTokens: 2048,
      maxOutputTokens: 512,
    ).stream(_request()).toList();

    final error = (events.single as ModelFailedEvent).error;
    expect(error, isA<AnthropicProviderException>());
    expect(
      (error as AnthropicProviderException).message,
      'thinking budget must be less than max_tokens',
    );
    expect(requests, 0);
  });

  test('sends temperature when thinking is disabled', () async {
    final requests = <Map<String, Object?>>[];
    final server = await _startServer((request) async {
      requests.add(
        jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, Object?>,
      );
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":0}}}',
        '{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    await _provider(server).stream(_request(temperature: 0.7)).toList();

    expect(requests.single['temperature'], 0.7);
  });

  test('embeds resources as document blocks', () async {
    final requests = <Map<String, Object?>>[];
    final server = await _startServer((request) async {
      requests.add(
        jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, Object?>,
      );
      await _sendSse(request.response, [
        '{"type":"message_start","message":{"usage":{"input_tokens":1,"output_tokens":0}}}',
        '{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}',
        '{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}',
        '{"type":"message_stop"}',
      ]);
    });
    addTearDown(server.close);

    await _provider(server)
        .stream(
          _request(
            messages: const [
              ModelMessage(
                role: ModelMessageRole.user,
                content: [
                  TextContent('look'),
                  ResourceContent(
                    uri: 'file:///tmp/a.dart',
                    mimeType: 'text/x-dart',
                    text: 'void main() {}',
                  ),
                ],
              ),
            ],
          ),
        )
        .toList();

    final user = (requests.single['messages'] as List).first;
    expect((user as Map)['content'], [
      {'type': 'text', 'text': 'look'},
      {
        'type': 'document',
        'source': {
          'type': 'text',
          'media_type': 'text/x-dart',
          'data': 'void main() {}',
        },
      },
    ]);
  });
}

ModelRequest _request({List<ModelMessage>? messages, double? temperature}) =>
    ModelRequest(
      sessionId: SessionId('session-1'),
      turnId: TurnId('turn-1'),
      model: _modelRef,
      messages:
          messages ??
          const [
            ModelMessage(
              role: ModelMessageRole.user,
              content: [TextContent('Hi')],
            ),
          ],
      systemPrompt: 'Be concise.',
      tools: const [
        ToolDescriptor(
          name: 'lookup',
          description: 'Look something up.',
          inputSchema: {'type': 'object'},
        ),
      ],
      reasoningEffort: 'high',
      temperature: temperature,
    );

final _modelRef = ModelRef(
  providerId: ProviderId('anthropic'),
  modelId: ModelId('claude-sonnet'),
);

AnthropicProvider _provider(
  HttpServer server, {
  int thinkingBudgetTokens = 0,
  int maxOutputTokens = 0,
}) {
  return AnthropicProvider([
    AnthropicProviderConfiguration(
      id: _modelRef.providerId,
      baseUrl: Uri.parse('http://${server.address.host}:${server.port}'),
      apiKey: 'secret-key',
      models: [
        AnthropicModelConfiguration(
          descriptor: ModelDescriptor(
            ref: _modelRef,
            maxOutputTokens: maxOutputTokens,
            reasoningEfforts: const [ReasoningEffortOption(value: 'high')],
          ),
          thinkingBudgetTokens: thinkingBudgetTokens,
        ),
      ],
    ),
  ]);
}

Future<HttpServer> _startServer(
  Future<void> Function(HttpRequest request) handler,
) async {
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
  unawaited(server.forEach(handler));
  return server;
}

Future<void> _sendSse(HttpResponse response, List<String> payloads) async {
  response.headers.contentType = ContentType('text', 'event-stream');
  for (final payload in payloads) {
    response.write('data: $payload\n\n');
    await response.flush();
  }
  await response.close();
}

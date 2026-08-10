import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  test('streams Chat Completions text, reasoning, tools, and usage', () async {
    final requests = <Map<String, Object?>>[];
    final server = await _startServer((request) async {
      requests.add(
        jsonDecode(await utf8.decoder.bind(request).join())
            as Map<String, Object?>,
      );
      await _sendSse(request.response, [
        '{"choices":[{"delta":{"content":"Hello"}}]}',
        '{"choices":[{"delta":{"reasoning_content":"checking"}}]}',
        '{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"lookup","arguments":"{\\"q\\":\\"x\\"}"}}]}}]}',
        '{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}',
        '{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}',
        '[DONE]',
      ]);
    });
    addTearDown(server.close);

    final provider = _provider(server, OpenAIProtocol.chatCompletions);
    final events = await provider.stream(_request()).toList();

    expect(events.whereType<TextDeltaEvent>().single.delta, 'Hello');
    expect(events.whereType<ReasoningDeltaEvent>().single.delta, 'checking');
    final response = (events.last as ModelCompletedEvent).response;
    expect(response.stopReason, StopReason.toolUse);
    expect(response.toolCalls.single.arguments['q'], 'x');
    expect(response.usage.totalTokens, 5);
    expect(requests.single['stream'], isTrue);
    expect(requests.single['stream_options'], {'include_usage': true});
  });

  test('replays Responses continuation items for the same provider', () async {
    var callCount = 0;
    final inputs = <List<Object?>>[];
    final server = await _startServer((request) async {
      final body =
          jsonDecode(await utf8.decoder.bind(request).join())
              as Map<String, Object?>;
      inputs.add((body['input'] as List).cast<Object?>());
      callCount++;
      if (callCount == 1) {
        await _sendSse(request.response, [
          '{"type":"response.output_text.delta","delta":"Done"}',
          '{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call-2","name":"inspect","arguments":"{\\"path\\":\\".\\"}"}}',
          '{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6},"output":[{"type":"message","content":[{"type":"output_text","text":"Done"}]},{"type":"function_call","call_id":"call-2","name":"inspect","arguments":"{\\"path\\":\\".\\"}"}]}}',
        ]);
      } else {
        await _sendSse(request.response, [
          '{"type":"response.output_text.delta","delta":"Again"}',
          '{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":6,"output_tokens":1,"total_tokens":7},"output":[]}}',
        ]);
      }
    });
    addTearDown(server.close);

    final provider = _provider(server, OpenAIProtocol.responses);
    final first = await provider.stream(_request()).toList();
    final firstResponse = (first.last as ModelCompletedEvent).response;
    expect((firstResponse.content.single as TextContent).text, 'Done');
    expect(firstResponse.toolCalls.single.name, 'inspect');
    expect(firstResponse.continuation?.opaquePayload['protocol'], 'responses');

    final secondRequest = _request(
      messages: [
        ModelMessage(
          role: ModelMessageRole.assistant,
          content: const [TextContent('Done')],
          continuation: firstResponse.continuation,
        ),
        ModelMessage(
          role: ModelMessageRole.tool,
          toolCallId: ToolCallId('call-2'),
          toolOutput: 'files',
        ),
      ],
    );
    final second = await provider.stream(secondRequest).toList();
    expect(
      (second.last as ModelCompletedEvent).response.content.single,
      isA<TextContent>(),
    );
    expect(inputs[1].first, isA<Map<String, Object?>>());
    expect((inputs[1].first as Map<String, Object?>)['type'], 'function_call');
    expect(
      (inputs[1].last as Map<String, Object?>)['type'],
      'function_call_output',
    );
  });

  test('does not replay Responses items when the model changes', () async {
    var callCount = 0;
    final inputs = <List<Object?>>[];
    final server = await _startServer((request) async {
      final body =
          jsonDecode(await utf8.decoder.bind(request).join())
              as Map<String, Object?>;
      inputs.add((body['input'] as List).cast<Object?>());
      callCount++;
      if (callCount == 1) {
        await _sendSse(request.response, [
          '{"type":"response.output_item.done","item":{"type":"function_call","call_id":"call-3","name":"inspect","arguments":"{\\"path\\":\\".\\"}"}}',
          '{"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","call_id":"call-3","name":"inspect","arguments":"{\\"path\\":\\".\\"}"}]}}',
        ]);
      } else {
        await _sendSse(request.response, [
          '{"type":"response.output_text.delta","delta":"new model"}',
          '{"type":"response.completed","response":{"status":"completed","output":[]}}',
        ]);
      }
    });
    addTearDown(server.close);

    final provider = _provider(
      server,
      OpenAIProtocol.responses,
      additionalModelId: ModelId('other-model'),
    );
    final first = await provider.stream(_request()).toList();
    final continuation =
        (first.last as ModelCompletedEvent).response.continuation;
    await provider
        .stream(
          _request(
            model: ModelRef(
              providerId: _modelRef.providerId,
              modelId: ModelId('other-model'),
            ),
            messages: [
              ModelMessage(
                role: ModelMessageRole.assistant,
                continuation: continuation,
              ),
            ],
          ),
        )
        .toList();

    expect((inputs[1].first as Map<String, Object?>)['role'], 'assistant');
    expect((inputs[1].first as Map<String, Object?>)['type'], isNull);
  });

  test('cancellation interrupts a streaming request', () async {
    final server = await _startServer((request) async {
      request.response.headers.contentType = ContentType(
        'text',
        'event-stream',
      );
      request.response.write(
        'data: {"choices":[{"delta":{"content":"partial"}}]}\n\n',
      );
      await request.response.flush();
      await Future<void>.delayed(const Duration(seconds: 1));
      await request.response.close();
    });
    addTearDown(server.close);

    final token = CancellationToken();
    final stream = _provider(
      server,
      OpenAIProtocol.chatCompletions,
    ).stream(_request(cancellation: token));
    final events = <ModelStreamEvent>[];
    final done = stream.forEach(events.add);
    final stopwatch = Stopwatch()..start();
    unawaited(
      Future<void>.delayed(const Duration(milliseconds: 20), token.cancel),
    );
    await done;
    stopwatch.stop();
    expect(
      (events.last as ModelFailedEvent).error,
      isA<TurnCancelledException>(),
    );
    expect(stopwatch.elapsed, lessThan(const Duration(milliseconds: 500)));
  });

  test('does not duplicate Responses text when only items are sent', () async {
    final server = await _startServer((request) async {
      await _sendSse(request.response, [
        '{"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"Only item text"}]}}',
        '{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Only item text"}]}]}}',
      ]);
    });
    addTearDown(server.close);

    final provider = _provider(server, OpenAIProtocol.responses);
    final events = await provider.stream(_request()).toList();
    final response = (events.last as ModelCompletedEvent).response;
    expect((response.content.single as TextContent).text, 'Only item text');
  });

  test('cancelling the subscription closes the underlying request', () async {
    final connectionClosed = Completer<void>();
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(server.close);
    unawaited(
      server.forEach((request) async {
        request.response.headers.contentType = ContentType(
          'text',
          'event-stream',
        );
        request.response.write(
          'data: {"choices":[{"delta":{"content":"partial"}}]}\n\n',
        );
        await request.response.flush();
        try {
          final socket = await request.response.detachSocket().timeout(
            const Duration(seconds: 2),
          );
          unawaited(socket.done.then((_) => connectionClosed.complete()));
        } catch (_) {
          connectionClosed.complete();
        }
      }),
    );

    final subscription = _provider(
      server,
      OpenAIProtocol.chatCompletions,
    ).stream(_request()).listen((_) {});
    await Future<void>.delayed(const Duration(milliseconds: 50));
    await subscription.cancel();

    await connectionClosed.future.timeout(
      const Duration(seconds: 3),
      onTimeout: () => fail('subscription cancel did not close the connection'),
    );
  });

  test('surfaces HTTP errors as provider exceptions', () async {
    final server = await _startServer((request) async {
      request.response.statusCode = HttpStatus.badRequest;
      request.response.write('nope');
      await request.response.close();
    });
    addTearDown(server.close);

    final events = await _provider(
      server,
      OpenAIProtocol.chatCompletions,
    ).stream(_request()).toList();
    final error = (events.single as ModelFailedEvent).error;
    expect(error, isA<OpenAIProviderException>());
    expect((error as OpenAIProviderException).statusCode, 400);
  });

  test('retries a rate limit before the stream starts', () async {
    var attempts = 0;
    final server = await _startServer((request) async {
      attempts++;
      if (attempts == 1) {
        request.response.statusCode = HttpStatus.tooManyRequests;
        request.response.headers.add('retry-after', '0');
        request.response.write('rate limited');
        await request.response.close();
        return;
      }
      await _sendSse(request.response, [
        '{"choices":[{"delta":{"content":"ok"}}]}',
        '{"choices":[{"delta":{},"finish_reason":"stop"}]}',
        '[DONE]',
      ]);
    });
    addTearDown(server.close);

    final events = await _provider(
      server,
      OpenAIProtocol.chatCompletions,
    ).stream(_request()).toList();
    expect(attempts, 2);
    expect(
      (events.last as ModelCompletedEvent).response.stopReason,
      StopReason.endTurn,
    );
  });
}

ModelRequest _request({
  ModelRef? model,
  List<ModelMessage>? messages,
  CancellationToken? cancellation,
}) {
  return ModelRequest(
    sessionId: SessionId('session-1'),
    turnId: TurnId('turn-1'),
    model: model ?? _modelRef,
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
    cancellation: cancellation,
  );
}

final _modelRef = ModelRef(
  providerId: ProviderId('test-provider'),
  modelId: ModelId('test-model'),
);

OpenAICompatibleProvider _provider(
  HttpServer server,
  OpenAIProtocol protocol, {
  ModelId? additionalModelId,
}) {
  final models = [
    OpenAIModelConfiguration(
      descriptor: ModelDescriptor(
        ref: _modelRef,
        inputCapabilities: const {ModelInputCapability.text},
        reasoningEfforts: const [ReasoningEffortOption(value: 'high')],
      ),
    ),
    if (additionalModelId != null)
      OpenAIModelConfiguration(
        descriptor: ModelDescriptor(
          ref: ModelRef(
            providerId: _modelRef.providerId,
            modelId: additionalModelId,
          ),
          inputCapabilities: const {ModelInputCapability.text},
          reasoningEfforts: const [ReasoningEffortOption(value: 'high')],
        ),
      ),
  ];
  return OpenAICompatibleProvider([
    OpenAIProviderConfiguration(
      id: _modelRef.providerId,
      protocol: protocol,
      baseUrl: Uri.parse('http://${server.address.host}:${server.port}'),
      models: models,
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

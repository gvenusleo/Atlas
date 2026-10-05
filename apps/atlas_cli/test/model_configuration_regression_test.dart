import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

void main() {
  test(
    'Opus 4.5 effort uses manual thinking; Opus 4.6 uses adaptive',
    () async {
      final capture = await _Capture.open();
      for (final (id, mode) in [
        ('claude-opus-4-5', 'enabled'),
        ('claude-opus-4-5-20251101', 'enabled'),
        ('claude-opus-4-6', 'adaptive'),
      ]) {
        final config = parseConfig(
          jsonEncode({'defaultProvider': 'anthropic', 'defaultModel': id}),
          modelsText: jsonEncode({
            'providers': {
              'anthropic': {
                'baseUrl': '${capture.origin}/v1',
                'apiKey': 'test',
              },
            },
          }),
          environment: {},
        );
        final events = await capture.run(config, effort: 'high');
        expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
        final body = capture.bodies.last;
        expect((body['thinking'] as Map)['type'], mode, reason: id);
        expect(body['output_config'], {'effort': 'high'}, reason: id);
        if (mode == 'enabled') {
          expect(
            (body['thinking'] as Map)['budget_tokens'],
            greaterThanOrEqualTo(1024),
          );
        }
      }
    },
  );

  test('manual thinking reserves answer tokens and rejects undersized budgets before HTTP', () async {
    final capture = await _Capture.open();
    final config = _custom(
      capture,
      api: 'anthropic-messages',
      model: {'reasoning': true},
    );
    for (final effort in ['minimal', 'medium', 'high']) {
      final events = await capture.run(config, effort: effort);
      expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
      final body = capture.bodies.last;
      final thinking = (body['thinking'] as Map)['budget_tokens'] as int;
      expect(thinking, greaterThanOrEqualTo(1024));
      expect(
        (body['max_tokens'] as int) - thinking,
        greaterThanOrEqualTo(1024),
      );
    }
    for (final cap in [1, 512, 1024, 2047]) {
      final before = capture.bodies.length;
      final events = await capture.run(config, effort: 'high', budget: cap);
      expect(events.single, isA<ModelFailedEvent>());
      expect(
        (events.single as ModelFailedEvent).error,
        isA<AnthropicProviderException>(),
      );
      expect(capture.bodies.length, before);
    }
    final events = await capture.run(config, effort: 'high', budget: 2048);
    expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
    expect(capture.bodies.last['thinking'], {
      'type': 'enabled',
      'budget_tokens': 1024,
    });
  });

  test(
    'strict support only enables strict for eligible tool schemas',
    () async {
      final capture = await _Capture.open();
      const valid = ToolDescriptor(
        name: 'strict_ok',
        description: 'eligible',
        inputSchema: {
          'type': 'object',
          'properties': {
            'nested': {
              'type': 'object',
              'properties': {
                'value': {'type': 'string'},
              },
              'additionalProperties': false,
            },
          },
          'required': ['nested'],
          'additionalProperties': false,
        },
      );
      const nestedInvalid = ToolDescriptor(
        name: 'nested_invalid',
        description: 'ineligible',
        inputSchema: {
          'type': 'object',
          'properties': {
            'nested': {
              'type': 'object',
              'properties': {
                'n': {'type': 'integer', 'minimum': 1},
              },
              'additionalProperties': false,
            },
          },
          'additionalProperties': false,
        },
      );
      for (final supported in [true, false]) {
        final config = _custom(
          capture,
          api: 'anthropic-messages',
          model: {
            'compat': {'supportsStrictTools': supported},
          },
        );
        final events = await capture.run(
          config,
          tools: [ReadTool().descriptor, valid, nestedInvalid],
        );
        expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
        final tools = (capture.bodies.last['tools'] as List)
            .cast<Map<String, Object?>>();
        expect(tools[0].containsKey('strict'), isFalse);
        expect(tools[0]['input_schema'], ReadTool().descriptor.inputSchema);
        expect(tools[1]['strict'], supported ? true : null);
        expect(tools[2].containsKey('strict'), isFalse);
      }
    },
  );

  for (final api in ['openai-completions', 'openai-responses']) {
    test('$api applies off sampling to an unselected thinking level', () async {
      final capture = await _Capture.open();
      final config = _custom(
        capture,
        api: api,
        model: {
          'reasoning': true,
          'samplingParams': {'temperature': 1.0, 'top_p': 0.8},
          'samplingParamsByThinkingLevel': {
            'off': {'temperature': 0.2},
            'high': {'top_p': 0.95},
          },
        },
      );
      await capture.run(config);
      expect(capture.bodies.last['temperature'], 0.2);
      expect(capture.bodies.last['top_p'], 0.8);
      await capture.run(config, effort: 'high');
      expect(capture.bodies.last['top_p'], 0.95);
    });
  }

  test(
    'header overrides merge case-insensitively across all config layers',
    () async {
      final capture = await _Capture.open();
      final config = _custom(
        capture,
        model: {
          'headers': {'X-Account': 'model', 'X-Other': 'retained'},
        },
        provider: {
          'headers': {'X-Account': 'provider'},
        },
        overrides: {
          'headers': {'x-account': 'override'},
        },
      );
      final events = await capture.run(config);
      expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
      expect(capture.headers.last['x-account'], 'override');
      expect(capture.headers.last['x-other'], 'retained');
      expect(
        () => _custom(
          capture,
          model: {
            'headers': {'X-Account': 'one', 'x-account': 'two'},
          },
        ),
        throwsA(isA<ConfigLoadException>()),
      );
      expect(
        () => _custom(
          capture,
          overrides: {
            'headers': {'X-Account': 'one', 'x-account': 'two'},
          },
        ),
        throwsA(isA<ConfigLoadException>()),
      );
    },
  );

  test(
    'Anthropic appends only messages to versioned and custom API bases',
    () async {
      final capture = await _Capture.open();
      for (final path in ['/v1', '/proxy/v1/', '/vendor/api']) {
        final config = _custom(
          capture,
          api: 'anthropic-messages',
          provider: {'baseUrl': '${capture.origin}$path'},
        );
        final events = await capture.run(config);
        expect(events.whereType<ModelCompletedEvent>(), hasLength(1));
        expect(
          capture.paths.last,
          '${path.replaceFirst(RegExp(r'/+$'), '')}/messages',
        );
      }
      expect(
        providerPresets['anthropic']!.baseUrl,
        'https://api.anthropic.com/v1',
      );
    },
  );
}

AtlasConfig _custom(
  _Capture capture, {
  String api = 'openai-completions',
  Map<String, Object?> model = const {},
  Map<String, Object?> provider = const {},
  Map<String, Object?> overrides = const {},
}) => parseConfig(
  '{"defaultProvider":"relay","defaultModel":"m"}',
  modelsText: jsonEncode({
    'providers': {
      'relay': {
        'api': api,
        'baseUrl': '${capture.origin}/v1',
        'apiKey': 'test',
        ...provider,
        'models': [
          {'id': 'm', ...model},
        ],
        'modelOverrides': {'m': overrides},
      },
    },
  }),
  catalog: ModelCatalog({}),
  environment: {},
);

final class _Capture(final HttpServer server) {
  final bodies = <Map<String, Object?>>[];
  final headers = <Map<String, String>>[];
  final paths = <String>[];
  String get origin => 'http://127.0.0.1:${server.port}';

  static Future<_Capture> open() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(() => server.close(force: true));
    final capture = _Capture(server);
    unawaited(
      server.forEach((request) async {
        capture.bodies.add(
          jsonDecode(await utf8.decoder.bind(request).join())
              as Map<String, Object?>,
        );
        final headers = <String, String>{};
        request.headers.forEach((name, values) {
          headers[name] = values.join(',');
        });
        capture.headers.add(headers);
        capture.paths.add(request.uri.path);
        request.response.headers.contentType = ContentType(
          'text',
          'event-stream',
        );
        if (request.uri.path.endsWith('/messages')) {
          request.response.write(
            'data: {"type":"message_start","message":{"usage":{"input_tokens":1}}}\n\n'
            'data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}\n\n'
            'data: {"type":"message_stop"}\n\n',
          );
        } else if (request.uri.path.endsWith('/responses')) {
          request.response.write(
            'data: {"type":"response.completed","response":{"status":"completed","output":[]}}\n\n',
          );
        } else {
          request.response.write(
            'data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}\n\n'
            'data: [DONE]\n\n',
          );
        }
        await request.response.close();
      }),
    );
    return capture;
  }

  Future<List<ModelStreamEvent>> run(
    AtlasConfig config, {
    String? effort,
    int budget = 0,
    List<ToolDescriptor> tools = const [],
  }) async {
    final entry = config.providers.firstWhere(
      (p) => switch (p) {
        ConfiguredOpenAI(:final configuration) => configuration.models.any(
          (m) => m.descriptor.ref == config.defaultModel,
        ),
        ConfiguredAnthropic(:final configuration) => configuration.models.any(
          (m) => m.descriptor.ref == config.defaultModel,
        ),
      },
    );
    final http = DioHttpStreamClient();
    try {
      final provider = switch (entry) {
        ConfiguredOpenAI(:final configuration) => OpenAICompatibleProvider([
          configuration,
        ], httpClient: http),
        ConfiguredAnthropic(:final configuration) => AnthropicProvider([
          configuration,
        ], httpClient: http),
      };
      return await provider
          .stream(
            ModelRequest(
              sessionId: SessionId('s'),
              turnId: TurnId('t'),
              model: config.defaultModel,
              messages: const [
                ModelMessage(
                  role: ModelMessageRole.user,
                  content: [TextContent('hello')],
                ),
              ],
              reasoningEffort: effort,
              maxOutputTokens: budget,
              tools: tools,
            ),
          )
          .toList();
    } finally {
      http.close();
    }
  }
}

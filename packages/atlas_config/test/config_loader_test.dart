import 'dart:convert';
import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  test('one relay routes models through different APIs and endpoints', () {
    final config = _parse({
      'baseUrl': 'https://relay.example/v1',
      'api': 'openai-completions',
      'apiKey': r'${UNSET_KEY}',
      'headers': {'X-Account': 'first'},
      'models': [
        {'id': 'a'},
        {
          'id': 'vendor/b',
          'api': 'anthropic-messages',
          'baseUrl': 'https://claude.example/v1',
          'headers': {'x-account': 'second'},
        },
      ],
    });
    final first = config.providers.first as ConfiguredOpenAI;
    final second = config.providers.last as ConfiguredAnthropic;
    expect(first.configuration.protocol, OpenAIProtocol.chatCompletions);
    expect(first.configuration.baseUrl.toString(), 'https://relay.example/v1');
    expect(
      second.configuration.baseUrl.toString(),
      'https://claude.example/v1',
    );
    expect(
      second.configuration.models.single.descriptor.ref.modelId.value,
      'vendor/b',
    );
    expect(second.configuration.authentication!.headers, {
      'x-account': 'second',
    });
    expect(config.defaultModel.toString(), 'relay/a');
    // Missing credentials do not prevent catalog loading.
    expect(
      first.configuration.authentication!.resolve(),
      throwsA(isA<ProviderAuthException>()),
    );
  });

  test(
    'models replaces a definition; modelOverrides merges selected metadata',
    () {
      final catalog = ModelCatalog({
        'relay': [
          {
            'id': 'a',
            'name': 'Base',
            'api': 'openai-responses',
            'baseUrl': 'https://base.example',
            'input': ['text', 'image'],
            'contextWindow': 90000,
            'maxTokens': 12000,
            'compat': {'supportsStrictMode': true},
            'cost': {'input': 2, 'output': 8},
          },
        ],
      });
      final config = _parse({
        'baseUrl': 'https://proxy.example',
        'modelOverrides': {
          'a': {
            'contextWindow': 64000,
            'cost': {'input': 1},
          },
          'unknown': {'name': 'Ignored'},
        },
      }, catalog: catalog);
      final model = (config.providers.single as ConfiguredOpenAI).configuration;
      expect(model.baseUrl.toString(), 'https://proxy.example');
      expect(model.models.single.descriptor.contextWindow, 64000);
      expect(
        model.models.single.descriptor.inputCapabilities,
        contains(ModelInputCapability.image),
      );
      expect(model.models.single.options.cost, {'input': 1, 'output': 8});
      final replaced = _parse({
        'models': [
          {'id': 'a', 'name': 'New'},
        ],
      }, catalog: catalog);
      final definition =
          (replaced.providers.single as ConfiguredOpenAI).configuration;
      expect(definition.protocol, OpenAIProtocol.responses);
      expect(definition.models.single.descriptor.contextWindow, 128000);
      expect(definition.models.single.descriptor.inputCapabilities, {
        ModelInputCapability.text,
      });
    },
  );

  test(
    'custom models inherit API and address from the provider model list',
    () {
      final config = _parse({
        'models': [
          {
            'id': 'a',
            'api': 'openai-responses',
            'baseUrl': 'https://relay.example',
          },
          {'id': 'b'},
        ],
      });
      final second = config.providers.last as ConfiguredOpenAI;
      expect(second.configuration.protocol, OpenAIProtocol.responses);
      expect(second.configuration.baseUrl.toString(), 'https://relay.example');
    },
  );

  test('startup thinking preference is clamped to supported levels', () {
    final config = parseConfig(
      '{"defaultProvider":"relay","defaultModel":"a","defaultThinkingLevel":"max"}',
      modelsText: jsonEncode({
        'providers': {
          'relay': {
            ..._provider,
            'models': [
              {
                'id': 'a',
                'reasoning': true,
                'thinkingLevelMap': {'low': 'low', 'high': 'high'},
              },
            ],
          },
        },
      }),
      catalog: ModelCatalog({}),
    );
    final model = (config.providers.single as ConfiguredOpenAI)
        .configuration
        .models
        .single;
    expect(model.options.defaultThinkingLevel, 'high');
    expect(model.descriptor.reasoningEfforts.first.value, 'high');
  });

  test('built-in models need no models.json declaration', () {
    final config = parseConfig(
      '{"defaultProvider":"openai","defaultModel":"gpt-4o"}',
      environment: {},
    );
    expect(config.providers, isNotEmpty);
    final model = config.providers
        .whereType<ConfiguredOpenAI>()
        .expand((p) => p.configuration.models)
        .firstWhere((m) => m.descriptor.ref == config.defaultModel);
    expect(model.descriptor.contextWindow, 128000);
    expect(
      model.descriptor.inputCapabilities,
      contains(ModelInputCapability.image),
    );
  });

  test(
    'invalid cached model metadata cannot prevent configuration startup',
    () async {
      final dir = await Directory.systemTemp.createTemp('atlas_cache_startup_');
      addTearDown(() => dir.delete(recursive: true));
      final cache = File('${dir.path}/cache/model-catalog.json');
      await cache.parent.create();
      await cache.writeAsString(
        jsonEncode({
          'version': 1,
          'data': {
            for (final id in ['openai', 'anthropic'])
              id: {
                'models': {
                  'broken': {
                    'name': '',
                    'tool_call': true,
                    'limit': {'context': 32000, 'output': 4096},
                    'modalities': {
                      'input': ['text'],
                      'output': ['text'],
                    },
                  },
                },
              },
          },
        }),
      );
      await File(
        '${dir.path}/settings.json',
      ).writeAsString('{"defaultProvider":"openai","defaultModel":"gpt-4o"}');
      final config = loadConfig(dir, environment: {});
      expect(config.defaultModel.toString(), 'openai/gpt-4o');
      expect(
        config.providers.whereType<ConfiguredOpenAI>().any(
          (p) => p.configuration.models.any(
            (m) => m.descriptor.ref == config.defaultModel,
          ),
        ),
        isTrue,
      );
    },
  );

  test('JSON comments preserve URLs and do not permit trailing commas', () {
    final config = parseConfig(
      '''{
      // startup defaults
      "defaultProvider": "relay", /* comment */ "defaultModel": "a"
    }''',
      modelsText: '{"providers":{"relay":{"api":"openai-completions","baseUrl":"https://example.com","models":[{"id":"a"}]}}}',
      catalog: ModelCatalog({}),
    );
    expect(config.defaultModel.toString(), 'relay/a');
    expect(
      () => parseConfig('{"defaultProvider":"openai",}'),
      throwsA(isA<ConfigLoadException>()),
    );
  });

  test(
    'settings and MCP documents load independently with supplied HOME',
    () async {
      final dir = await Directory.systemTemp.createTemp('atlas_config_');
      addTearDown(() => dir.delete(recursive: true));
      await File('${dir.path}/settings.json').writeAsString(
        jsonEncode({
          'defaultProvider': 'relay',
          'defaultModel': 'a',
          'defaultThinkingLevel': 'high',
          'agent': {'maxSteps': 3, 'maxOutputTokens': 2048, 'temperature': 0.2},
          'compaction': {'keepRecentTokens': 8000, 'reserveTokens': 4096},
          'session': {'dbPath': '~/data/atlas.db'},
          'logging': {'directory': '~/logs'},
        }),
      );
      await File('${dir.path}/models.json').writeAsString(
        jsonEncode({
          'providers': {'relay': _provider},
        }),
      );
      await File('${dir.path}/mcp.json')
          .writeAsString('{"mcpServers":{"local":{"command":"server"}}}');
      final config = loadConfig(
        dir,
        environment: {'HOME': dir.path, 'ATLAS_LOG_LEVEL': 'debug'},
      );
      expect(config.session.dbPath, '${dir.path}/data/atlas.db');
      expect(config.logging.level, 'debug');
      expect(config.agent.maxSteps, 3);
      expect(config.agent.compaction.reserveTokens, 4096);
      expect(config.mcpServers.single.name, 'local');
      await File('${dir.path}/config.yaml').writeAsString('legacy');
      expect(
        () => loadConfig(dir),
        throwsA(
          isA<ConfigLoadException>().having(
            (e) => e.message,
            'message',
            contains('no longer supported'),
          ),
        ),
      );
    },
  );

  test('thinking and sampling overrides retain their separate meanings', () {
    final config = _parse({
      ..._provider,
      'models': [
        {
          'id': 'a',
          'reasoning': true,
          'thinkingLevelMap': {'off': null, 'high': 'max'},
          'samplingParams': {'temperature': 0.7},
          'samplingParamsByThinkingLevel': {
            'high': {'top_p': 0.9},
          },
        },
      ],
    });
    final options = (config.providers.single as ConfiguredOpenAI)
        .configuration
        .models
        .single
        .options;
    expect(options.effort('high'), 'max');
    expect(options.effort('off'), isNull);
    expect(options.sampling('high'), {'temperature': 0.7, 'top_p': 0.9});
  });

  for (final (label, provider, path)
      in <(String, Map<String, Object?>, String)>[
        ('unknown API', {..._provider, 'api': 'google-generative-ai'}, '.api'),
        (
          'unsupported compat',
          {
            ..._provider,
            'compat': {'magic': true},
          },
          '.compat.magic',
        ),
        (
          'unsupported reasoning format',
          {
            ..._provider,
            'compat': {'thinkingFormat': 'qwen'},
          },
          '.compat.thinkingFormat',
        ),
        (
          'duplicate model',
          {
            ..._provider,
            'models': [
              {'id': 'a'},
              {'id': 'a'},
            ],
          },
          '.id',
        ),
        (
          'zero context',
          {
            ..._provider,
            'models': [
              {'id': 'a', 'contextWindow': 0},
            ],
          },
          '.contextWindow',
        ),
        (
          'negative output',
          {
            ..._provider,
            'models': [
              {'id': 'a', 'maxTokens': -1},
            ],
          },
          '.maxTokens',
        ),
        (
          'header injection',
          {
            ..._provider,
            'headers': {'X-Test': 'a\nb'},
          },
          '.headers',
        ),
        (
          'URL credentials',
          {..._provider, 'baseUrl': 'https://user:secret@example.com'},
          '.baseUrl',
        ),
        (
          'request override',
          {
            ..._provider,
            'models': [
              {
                'id': 'a',
                'samplingParams': {'model': 'other'},
              },
            ],
          },
          '.samplingParams.model',
        ),
      ]) {
    test('rejects $label with a field path', () {
      expect(
        () => _parse(provider),
        throwsA(
          isA<ConfigLoadException>().having(
            (e) => e.message,
            'message',
            contains(path),
          ),
        ),
      );
    });
  }

  test(
    'unknown default and unsupported provider options are explicit errors',
    () {
      expect(
        () => parseConfig('{"defaultProvider":"missing","defaultModel":"x"}'),
        throwsA(isA<ConfigLoadException>()),
      );
      expect(
        () => _parse({..._provider, 'oauth': 'radius'}),
        throwsA(isA<ConfigLoadException>()),
      );
    },
  );
}

const _provider = <String, Object?>{
  'api': 'openai-completions',
  'baseUrl': 'https://example.com',
  'apiKey': 'test',
  'models': [
    {'id': 'a'},
  ],
};

AtlasConfig _parse(Map<String, Object?> provider, {ModelCatalog? catalog}) =>
    parseConfig(
      '{"defaultProvider":"relay","defaultModel":"a"}',
      modelsText: jsonEncode({
        'providers': {'relay': provider},
      }),
      environment: {},
      catalog: catalog ?? ModelCatalog({}),
    );

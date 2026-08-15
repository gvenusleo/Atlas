import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  const env = {
    'ANTHROPIC_API_KEY': 'sk-ant-test',
    'OPENAI_API_KEY': 'sk-oa-test',
  };
  const home = '/home/test';

  test('parses a full configuration with both provider types', () {
    final config = parseConfig(
      '''
default_model: anthropic/claude-sonnet
providers:
  - name: anthropic
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: \${ANTHROPIC_API_KEY}
    api_version: "2023-06-01"
    models:
      - value: claude-sonnet
        name: Claude Sonnet
        context_window: 200000
        max_tokens: 4096
        thinking_budget_tokens: 2048
        reasoning_efforts:
          - value: high
  - name: openai
    type: responses
    base_url: https://api.openai.com/v1
    api_key: \${OPENAI_API_KEY}
    user_agent: Atlas
    models:
      - value: gpt-4o
        context_window: 128000
        max_tokens: 4096
        input_capabilities: [text, image]
        prompt_cache: true
agent:
  max_steps: 10
  temperature: 0.7
  compaction:
    threshold: 0.9
session:
  db_path: ~/.atlas/atlas.db
''',
      environment: env,
      homeDirectory: home,
    );

    expect(
      config.defaultModel,
      ModelRef(
        providerId: ProviderId('anthropic'),
        modelId: ModelId('claude-sonnet'),
      ),
    );
    expect(config.providers, hasLength(2));

    final anthropic = config.providers.first as ConfiguredAnthropic;
    expect(anthropic.configuration.apiKey, 'sk-ant-test');
    expect(anthropic.configuration.apiVersion, '2023-06-01');
    expect(anthropic.configuration.models.single.thinkingBudgetTokens, 2048);
    expect(
      anthropic.configuration.models.single.descriptor.name,
      'Claude Sonnet',
    );
    expect(
      anthropic
          .configuration
          .models
          .single
          .descriptor
          .reasoningEfforts
          .single
          .value,
      'high',
    );

    final openai = config.providers.last as ConfiguredOpenAI;
    expect(openai.configuration.protocol, OpenAIProtocol.responses);
    expect(openai.configuration.userAgent, 'Atlas');
    expect(openai.configuration.models.single.promptCacheEnabled, isTrue);
    expect(openai.configuration.models.single.descriptor.inputCapabilities, {
      ModelInputCapability.text,
      ModelInputCapability.image,
    });

    expect(config.agent.maxSteps, 10);
    expect(config.agent.temperature, 0.7);
    expect(config.agent.compaction.threshold, 0.9);
    expect(config.session.dbPath, '/home/test/.atlas/atlas.db');
  });

  test('applies defaults for omitted fields', () {
    final config = parseConfig(
      '''
default_model: oa/gpt-4o
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: \${KEY}
    models:
      - value: gpt-4o
''',
      environment: const {'KEY': 'v'},
    );

    final openai = config.providers.single as ConfiguredOpenAI;
    expect(openai.configuration.userAgent, isNull);
    final descriptor = openai.configuration.models.single.descriptor;
    expect(descriptor.name, '');
    expect(descriptor.contextWindow, 0);
    expect(descriptor.maxOutputTokens, 0);
    expect(descriptor.inputCapabilities, const {ModelInputCapability.text});
    expect(descriptor.reasoningEfforts, isEmpty);
    expect(openai.configuration.models.single.promptCacheEnabled, isFalse);
    expect(config.agent.maxSteps, 100);
    expect(config.agent.compaction.threshold, 0.8);
    expect(config.agent.temperature, isNull);
    expect(config.session.dbPath, '~/.atlas/atlas.db');
  });

  test('rejects zero max_steps', () {
    expect(
      () => parseConfig('''
default_model: oa/gpt-4o
providers:
  - name: oa
    type: responses
    base_url: https://example.com
    api_key: key
    models:
      - value: gpt-4o
agent:
  max_steps: 0
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('agent.max_steps must be greater than zero'),
        ),
      ),
    );
  });

  test('rejects an Anthropic thinking budget at max_tokens', () {
    expect(
      () => parseConfig('''
default_model: anthropic/claude
providers:
  - name: anthropic
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: key
    models:
      - value: claude
        max_tokens: 2048
        thinking_budget_tokens: 2048
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('thinking_budget_tokens must be less than'),
        ),
      ),
    );
  });

  test('expands the home directory only for a leading tilde', () {
    final config = parseConfig('''
default_model: oa/gpt-4o
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: k
    models:
      - value: gpt-4o
session:
  db_path: /var/data/atlas.db
''', homeDirectory: home);
    expect(config.session.dbPath, '/var/data/atlas.db');
  });

  test('rejects an undefined environment variable with its name', () {
    expect(
      () => parseConfig('''
default_model: oa/gpt-4o
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: \${MISSING_KEY}
    models:
      - value: gpt-4o
''', environment: const {}),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('MISSING_KEY'),
        ),
      ),
    );
  });

  test('rejects duplicate provider names', () {
    expect(
      () => parseConfig('''
default_model: a/m
providers:
  - name: a
    type: chat_completions
    base_url: https://one.example.com
    api_key: k
    models:
      - value: m
  - name: a
    type: chat_completions
    base_url: https://two.example.com
    api_key: k
    models:
      - value: m
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('providers[1].name'),
        ),
      ),
    );
  });

  test('rejects an unknown type with a field path', () {
    expect(
      () => parseConfig('''
default_model: a/m
providers:
  - name: a
    type: llama
    base_url: https://example.com
    api_key: k
    models:
      - value: m
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('providers[0].type'),
        ),
      ),
    );
  });

  test('rejects a default model that references an unknown provider', () {
    expect(
      () => parseConfig('''
default_model: missing/gpt-4o
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: k
    models:
      - value: gpt-4o
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('unknown provider "missing"'),
        ),
      ),
    );
  });

  test('rejects a default model that references an unknown model', () {
    expect(
      () => parseConfig('''
default_model: oa/nope
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: k
    models:
      - value: gpt-4o
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('unknown model "nope"'),
        ),
      ),
    );
  });

  test('rejects an invalid base URL', () {
    expect(
      () => parseConfig('''
default_model: oa/m
providers:
  - name: oa
    type: chat_completions
    base_url: ftp://example.com
    api_key: k
    models:
      - value: m
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('providers[0].base_url'),
        ),
      ),
    );
  });

  test('rejects duplicate model ids within a provider', () {
    expect(
      () => parseConfig('''
default_model: oa/m
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: k
    models:
      - value: m
      - value: m
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('duplicates model "m"'),
        ),
      ),
    );
  });

  test('rejects a negative max_tokens value', () {
    expect(
      () => parseConfig('''
default_model: oa/m
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: k
    models:
      - value: m
        max_tokens: -1
'''),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('max_tokens'),
        ),
      ),
    );
  });

  test('rejects invalid YAML syntax', () {
    expect(
      () => parseConfig('providers: [unclosed'),
      throwsA(isA<ConfigLoadException>()),
    );
  });

  test('rejects an empty providers list', () {
    expect(
      () => parseConfig('default_model: a/m\nproviders: []'),
      throwsA(
        isA<ConfigLoadException>().having(
          (error) => error.message,
          'message',
          contains('providers must not be empty'),
        ),
      ),
    );
  });

  test('rejects an invalid compaction threshold', () {
    for (final threshold in ['0', '-0.5', '1.5']) {
      expect(
        () => parseConfig('''
default_model: oa/m
providers:
  - name: oa
    type: chat_completions
    base_url: https://example.com
    api_key: k
    models:
      - value: m
agent:
  compaction:
    threshold: $threshold
'''),
        throwsA(
          isA<ConfigLoadException>().having(
            (error) => error.message,
            'message',
            contains('agent.compaction.threshold'),
          ),
        ),
      );
    }
  });
}

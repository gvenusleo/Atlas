import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:test/test.dart';

void main() {
  test('parses cache options', () {
    expect(parseCacheOptions(const []).limit, 200);
    expect(parseCacheOptions(const ['--limit', '5']).limit, 5);
    expect(() => parseCacheOptions(const ['--limit']), throwsFormatException);
    expect(
      () => parseCacheOptions(const ['--limit', 'zero']),
      throwsFormatException,
    );
    expect(() => parseCacheOptions(const ['--unknown']), throwsFormatException);
  });

  test('counts every model request of a sampled turn', () async {
    final store = DriftSessionStore.inMemory();
    addTearDown(store.close);
    await _insertTurn(
      store,
      sessionId: 'session-claude',
      turnId: 'turn-1',
      sequence: 1,
      providerId: 'claude',
      title: 'Claude session',
      steps: const [
        TokenUsage(
          inputTokens: 100,
          outputTokens: 10,
          totalTokens: 110,
          cacheWriteInputTokens: 50,
        ),
        TokenUsage(
          inputTokens: 150,
          outputTokens: 10,
          totalTokens: 160,
          cacheReadInputTokens: 100,
        ),
      ],
    );

    final out = StringBuffer();
    final code = await runCacheCommand(store, config: _config(), out: out);

    expect(code, 0);
    final text = out.toString();
    // Both requests count: 100 read over (100 + 50) + (150 + 100) prompt.
    expect(text, contains('25.0% hit rate'));
    expect(text, contains('Sampled 2 model requests from 1 turn in 1 session'));
    // The turn row keeps only the last request, so a turn-based report would
    // have printed 40.0% here.
    expect(text, isNot(contains('40.0%')));
    expect(text, contains('Claude session'));
  });

  test('counts OpenAI-style cached tokens inside input tokens', () async {
    final store = DriftSessionStore.inMemory();
    addTearDown(store.close);
    await _insertTurn(
      store,
      sessionId: 'session-local',
      turnId: 'turn-1',
      sequence: 1,
      providerId: 'local',
      steps: const [
        TokenUsage(inputTokens: 1000, cacheReadInputTokens: 400),
        TokenUsage(inputTokens: 2000, cacheReadInputTokens: 1000),
      ],
    );

    final out = StringBuffer();
    await runCacheCommand(store, config: _config(), out: out);

    final text = out.toString();
    // Cached tokens are already part of input_tokens: 1400 / 3000.
    expect(text, contains('46.7% hit rate'));
    expect(
      text,
      contains(
        'Cache reads appeared in 2 of 2 model requests that reported usage.',
      ),
    );
    expect(text, contains('Cache reuse is partial'));
  });

  test('points at requests that missed the cache', () async {
    final store = DriftSessionStore.inMemory();
    addTearDown(store.close);
    await _insertTurn(
      store,
      sessionId: 'session-local',
      turnId: 'turn-1',
      sequence: 1,
      providerId: 'local',
      steps: const [
        TokenUsage(inputTokens: 500),
        TokenUsage(inputTokens: 500, cacheReadInputTokens: 500),
      ],
    );

    final out = StringBuffer();
    await runCacheCommand(store, config: _config(), out: out);

    expect(out.toString(), contains('Cache reads appeared in 1 of 2'));
    expect(
      out.toString(),
      contains('Requests without a cache read are the ones to inspect first'),
    );
  });

  test(
    'leaves requests without reported usage out of the request counts',
    () async {
      final store = DriftSessionStore.inMemory();
      addTearDown(store.close);
      await _insertTurn(
        store,
        sessionId: 'session-claude',
        turnId: 'turn-1',
        sequence: 1,
        providerId: 'claude',
        steps: const [
          TokenUsage(inputTokens: 100, cacheReadInputTokens: 900),
          TokenUsage(),
        ],
      );

      final out = StringBuffer();
      await runCacheCommand(store, config: _config(), out: out);

      final text = out.toString();
      // The empty step is inspected but reported nothing, so it is not a miss.
      expect(text, contains('Sampled 2 model requests'));
      expect(
        text,
        contains(
          'Cache reads appeared in 1 of 1 model requests that reported usage.',
        ),
      );
      expect(text, contains('reported no token usage'));
      expect(text, contains('90.0% hit rate'));
    },
  );

  test('reclassifies providers that report Anthropic-style numbers', () async {
    final store = DriftSessionStore.inMemory();
    addTearDown(store.close);
    await _insertTurn(
      store,
      sessionId: 'session-proxy',
      turnId: 'turn-1',
      sequence: 1,
      // Configured as OpenAI-compatible, but the numbers are Anthropic-shaped.
      providerId: 'local',
      steps: const [TokenUsage(inputTokens: 100, cacheReadInputTokens: 900)],
    );

    final out = StringBuffer();
    await runCacheCommand(store, config: _config(), out: out);

    final text = out.toString();
    // Cache reads sit outside input_tokens: 900 / (100 + 900), never above 100%.
    expect(text, contains('90.0% hit rate'));
    expect(text, contains('\nNotes\n'));
    expect(text, contains('reports cache reads outside input_tokens'));
    expect(text, isNot(contains('900.0%')));
  });

  test(
    'falls back to recorded numbers for providers no longer configured',
    () async {
      final store = DriftSessionStore.inMemory();
      addTearDown(store.close);
      await _insertTurn(
        store,
        sessionId: 'session-gone',
        turnId: 'turn-1',
        sequence: 1,
        providerId: 'retired',
        steps: const [
          TokenUsage(
            inputTokens: 50,
            cacheReadInputTokens: 150,
            cacheWriteInputTokens: 10,
          ),
        ],
      );

      final out = StringBuffer();
      await runCacheCommand(store, config: _config(), out: out);

      // A cache write marks Anthropic accounting: 150 / (50 + 150 + 10).
      expect(out.toString(), contains('71.4% hit rate'));
    },
  );

  test('honors the turn limit and reports an empty database', () async {
    final empty = DriftSessionStore.inMemory();
    final emptyOut = StringBuffer();
    expect(await runCacheCommand(empty, config: _config(), out: emptyOut), 0);
    expect(emptyOut.toString(), contains('No turns recorded yet'));
    await empty.close();

    final store = DriftSessionStore.inMemory();
    addTearDown(store.close);
    for (var index = 1; index <= 3; index++) {
      await _insertTurn(
        store,
        sessionId: 'session-$index',
        turnId: 'turn-$index',
        sequence: 1,
        providerId: 'local',
        startedAt: DateTime.utc(2026, 9, index),
        steps: const [TokenUsage(inputTokens: 100)],
      );
    }

    final out = StringBuffer();
    await runCacheCommand(
      store,
      config: _config(),
      options: const CacheOptions(limit: 1),
      out: out,
    );
    expect(out.toString(), contains('Sampled 1 model request from 1 turn'));
  });

  test('rejects unknown options on stderr without opening storage', () async {
    final out = StringBuffer();
    final err = StringBuffer();
    final runner = AtlasCommandRunner(
      out: out,
      err: err,
      configLoader: () => throw StateError('must not load configuration'),
    );
    final code = await runCli(['cache', '--nope'], runner: runner);

    expect(code, 64);
    expect(out.toString(), isEmpty);
    expect(err.toString(), contains('Usage: atlas cache'));
    expect(err.toString(), contains('nope'));
  });
}

/// A configuration with one Anthropic and one OpenAI-compatible provider.
AtlasConfig _config() => parseConfig('''
default_model: claude/claude-sonnet
providers:
  - name: claude
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: test-key
    models:
      - value: claude-sonnet
  - name: local
    type: chat_completions
    base_url: https://example.test/v1
    api_key: test-key
    models:
      - value: gpt-local
''');

/// Inserts one completed turn whose model requests carry [steps] usage.
Future<void> _insertTurn(
  DriftSessionStore store, {
  required String sessionId,
  required String turnId,
  required int sequence,
  required String providerId,
  required List<TokenUsage> steps,
  String title = '',
  DateTime? startedAt,
}) async {
  final session = SessionId(sessionId);
  final turn = TurnId(turnId);
  final started = startedAt ?? DateTime.utc(2026, 9, 1);
  final model = ModelRef(
    providerId: ProviderId(providerId),
    modelId: ModelId('model-1'),
  );
  await store.beginTurn(
    BeginTurn(
      session: Session(
        id: session,
        title: title,
        workingDirectory: '/tmp',
        createdAt: started,
        updatedAt: started,
      ),
      turn: Turn(
        id: turn,
        sessionId: session,
        status: TurnStatus.running,
        startedAt: started,
        model: model,
      ),
      userMessage: UserMessageItem(
        id: TimelineItemId('$turnId-message'),
        sessionId: session,
        turnId: turn,
        sequence: sequence,
        occurredAt: started,
        content: const [TextContent('hi')],
      ),
    ),
  );
  for (var index = 0; index < steps.length; index++) {
    await store.appendModelStep(
      session,
      PersistedModelStep(
        assistantMessage: AssistantMessageItem(
          id: TimelineItemId('$turnId-step-$index'),
          sessionId: session,
          turnId: turn,
          sequence: sequence + index + 1,
          occurredAt: started.add(Duration(seconds: index)),
          content: [TextContent('step $index')],
          model: model,
          stopReason: StopReason.endTurn,
          usage: steps[index],
        ),
        toolCalls: const [],
      ),
    );
  }
  await store.finishTurn(
    session,
    Turn(
      id: turn,
      sessionId: session,
      status: TurnStatus.completed,
      startedAt: started,
      completedAt: started.add(const Duration(minutes: 1)),
      model: model,
      usage: steps.last,
    ),
  );
}

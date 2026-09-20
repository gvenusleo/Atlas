import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:test/test.dart';

void main() {
  late DriftSessionStore store;
  setUp(() => store = DriftSessionStore.inMemory());
  tearDown(() => store.close());

  test('validates cache options', () {
    expect(parseCacheOptions(const []).limit, 200);
    expect(parseCacheOptions(const ['--limit', '5']).limit, 5);
    for (final args in [
      ['--limit'],
      ['--limit', 'zero'],
      ['--limit', '0'],
      ['--limit', '-1'],
      ['--unknown'],
      ['extra'],
    ]) {
      expect(() => parseCacheOptions(args), throwsFormatException);
    }
  });

  test(
    'weights every request by its complete input, not its turn or ratio',
    () async {
      await _insertTurn(
        store,
        steps: [_usage(150, write: 50), _usage(250, read: 100)],
      );
      final text = await _report(store);
      expect(text, contains('2 recorded requests'));
      expect(text, contains('25.0%'));
      expect(text, contains('100 / 400 measured input tokens'));
      expect(text, contains('50.0%'));
      expect(text, contains('1 / 2 measured requests'));
      expect(text, contains('2 / 2 recorded requests'));
      expect(text, isNot(contains('40.0%')));
      expect(text, isNot(contains('healthy')));
      expect(text, contains('test / model-1'));
      expect(text, contains('Cache test'));
    },
  );

  test('counts OpenAI subsets without adding cached tokens again', () async {
    await _insertTurn(
      store,
      steps: [_usage(1000, read: 400), _usage(2000, read: 1000)],
    );
    final text = await _report(store);
    expect(text, contains('46.7%'));
    expect(text, contains('1,400 / 3,000 measured input tokens'));
    expect(text, contains('2 / 2 measured requests'));
  });

  test('cache writes are not reads', () async {
    await _insertTurn(store, steps: [_usage(12000, write: 12000)]);
    final text = await _report(store);
    expect(text, contains('0 / 12,000 measured input tokens'));
    expect(text, contains('0 / 1 measured requests'));
    expect(text, contains(RegExp(r'Cache write\s+12,000')));
    expect(text, contains(RegExp(r'Fresh input\s+0')));
    expect(text, isNot(contains('never read back')));
  });

  test(
    'legacy, missing cache fields and aborted usage do not become misses',
    () async {
      await _insertTurn(
        store,
        steps: [
          _usage(1000, read: 900),
          const TokenUsage(inputTokens: 10000, cacheReadInputTokens: 5000),
          const TokenUsage(inputTokens: 10000, promptTokens: 10000),
          const TokenUsage(),
        ],
      );
      // Historical aborted rows may contain copied usage. Ignore even fully
      // populated usage on these rows instead of repeating the preceding call.
      await _insertTurn(
        store,
        turnId: 'turn-2',
        sessionId: 'session-2',
        steps: [_usage(9000)],
        stopReason: StopReason.aborted,
      );
      final text = await _report(store);
      expect(text, contains('90.0%'));
      expect(text, contains('900 / 1,000 measured input tokens'));
      expect(text, contains('20.0%'));
      expect(text, contains('1 / 5 recorded requests'));
      expect(text, contains('3 requests excluded: cache fields'));
      expect(text, contains('1 aborted response excluded'));
    },
  );

  test('unknown is n/a while explicitly reported zero is a miss', () async {
    await _insertTurn(store, steps: [const TokenUsage(inputTokens: 1000)]);
    final unknown = await _report(store);
    expect(unknown, contains(RegExp(r'Token hit rate\s+n/a')));
    expect(unknown, contains('0 / 1 recorded requests'));
    await _insertTurn(
      store,
      turnId: 'turn-2',
      sessionId: 'session-2',
      steps: [_usage(1000)],
    );
    final measured = await _report(store);
    expect(measured, contains(RegExp(r'Token hit rate\s+0\.0%')));
    expect(measured, contains('1 / 2 recorded requests'));
  });

  test(
    'rejects inconsistent usage instead of guessing additive accounting',
    () async {
      await _insertTurn(
        store,
        steps: [
          _usage(100, read: 900),
          _usage(100, read: 80, write: 30),
          _usage(-1),
          _usage(100, read: -1),
          _usage(100, write: -1),
          _usage(0),
          _usage(1000, read: 400),
        ],
      );
      final text = await _report(store);
      expect(text, contains('5 requests excluded: inconsistent token counts'));
      expect(text, contains('1 request excluded: zero input tokens'));
      expect(text, contains('400 / 1,000 measured input tokens'));
      expect(text, contains('1 / 7 recorded requests'));
    },
  );

  test(
    'unreported writes stay unclassified rather than becoming fresh',
    () async {
      await _insertTurn(
        store,
        steps: [
          const TokenUsage(
            inputTokens: 1000,
            promptTokens: 1000,
            cacheReadInputTokens: 400,
            cacheReadReported: true,
          ),
        ],
      );
      final text = await _report(store);
      expect(text, contains('40.0%'));
      expect(text, contains(RegExp(r'Unclassified input\s+600')));
      expect(text, contains(RegExp(r'Fresh input\s+0 \(known\)')));
      expect(text, contains('fresh/write split unknown'));
    },
  );

  test('compaction cannot change historical counts or rates', () async {
    await _insertTurn(store, steps: [_usage(1000, read: 400)]);
    final before = await _report(store);
    await store.saveCompaction(
      SessionId('session-1'),
      CompactionCheckpoint(
        sessionId: SessionId('session-1'),
        compactedThroughSequence: 2,
        summary: 'summary',
        keptRecentMessages: 0,
        inputTokensBefore: 1000,
        inputTokensAfter: 10,
        createdAt: DateTime.utc(2026, 9, 2),
      ),
    );
    expect((await store.loadSession(SessionId('session-1'))).timeline, isEmpty);
    expect(await _report(store), before);
    final usage = (await store.recentTurnUsage()).single.requests.single.usage;
    expect(usage.promptTokens, 1000);
    expect(usage.cacheReadReported, isTrue);
    expect(usage.cacheWriteReported, isTrue);
  });

  test(
    'normalized accounting survives provider retirement without inference',
    () async {
      await _insertTurn(
        store,
        providerId: 'retired',
        steps: [
          const TokenUsage(
            inputTokens: 1000,
            promptTokens: 1400,
            cacheReadInputTokens: 400,
            cacheReadReported: true,
            cacheWriteReported: true,
          ),
        ],
      );
      final text = await _report(store);
      expect(text, contains('28.6%'));
      expect(text, contains('400 / 1,400 measured input tokens'));
      expect(text, contains('retired / model-1'));
    },
  );

  test(
    'groups by response model and preserves sampled turns with no responses',
    () async {
      await _insertTurn(store, steps: [_usage(1000, read: 400)]);
      await _insertTurn(
        store,
        sessionId: 'session-2',
        turnId: 'turn-2',
        modelId: 'model-2',
        steps: [_usage(1000)],
      );
      await _insertTurn(
        store,
        sessionId: 'session-3',
        turnId: 'turn-3',
        steps: [],
      );
      final text = await _report(store);
      expect(text, contains('3 turns | 3 sessions | 2 recorded requests'));
      expect(text, contains('test / model-1'));
      expect(text, contains('test / model-2'));
      expect(text, contains('session-3'));
      expect(text, contains('attempts without a response record'));
    },
  );

  test(
    'honors newest-turn limit with stable ties and reports empty storage',
    () async {
      expect(await _report(store), contains('No turns recorded yet'));
      for (var index = 1; index <= 3; index++) {
        await _insertTurn(
          store,
          sessionId: 'session-$index',
          turnId: 'turn-$index',
          steps: [_usage(1000, read: index * 100)],
        );
      }
      final text = await _report(store, limit: 1);
      expect(text, contains('Latest 1 turn | 1 session | 1 recorded request'));
      expect(text, contains('30.0%'));
      expect(text, contains('session-3'));
      expect(text, isNot(contains('session-1')));
    },
  );

  test(
    'large turn limits exceed neither SQL parameter bounds nor sample scope',
    () async {
      for (var index = 0; index < 501; index++) {
        await _insertTurn(
          store,
          sessionId: 'session-$index',
          turnId: 'turn-$index',
          steps: [_usage(1)],
        );
      }
      final samples = await store.recentTurnUsage(limit: 1000);
      expect(samples, hasLength(501));
      expect(samples.expand((sample) => sample.requests), hasLength(501));
    },
  );

  test(
    'plain output bounds long names and CJK titles without terminal controls',
    () async {
      await _insertTurn(
        store,
        title: '${'终端报告' * 30}\x1b[31m\n\tunsafe\u202e',
        providerId: 'a' * 80,
        modelId: 'b' * 80,
        steps: [_usage(1000, read: 400)],
      );
      for (final width in [1, 32, 60, 80, 120]) {
        final text = await _report(store, columns: width);
        expect(text, isNot(contains('\x1b')));
        expect(text, isNot(contains('\t')));
        expect(text, isNot(contains('\u202e')));
        for (final line in text.split('\n')) {
          final cells = line.runes.fold(
            0,
            (n, rune) => n + (rune < 128 ? 1 : 2),
          );
          expect(cells, lessThanOrEqualTo(width), reason: line);
        }
      }
    },
  );

  test('rejects unknown options before opening storage', () async {
    final out = StringBuffer();
    final err = StringBuffer();
    final code = await runCli(
      ['cache', '--nope'],
      runner: AtlasCommandRunner(
        out: out,
        err: err,
        configLoader: () => throw StateError('must not load configuration'),
      ),
    );
    expect(code, 64);
    expect(out.toString(), isEmpty);
    expect(err.toString(), contains('Usage: atlas cache'));
  });
}

TokenUsage _usage(int prompt, {int read = 0, int write = 0}) => TokenUsage(
  inputTokens: prompt,
  promptTokens: prompt,
  cacheReadInputTokens: read,
  cacheWriteInputTokens: write,
  cacheReadReported: true,
  cacheWriteReported: true,
);

Future<String> _report(
  DriftSessionStore store, {
  int limit = 200,
  int columns = 120,
}) async {
  final out = StringBuffer();
  await runCacheCommand(
    store,
    options: CacheOptions(limit: limit),
    out: out,
    columns: columns,
  );
  return out.toString();
}

Future<void> _insertTurn(
  DriftSessionStore store, {
  String sessionId = 'session-1',
  String turnId = 'turn-1',
  String providerId = 'test',
  String modelId = 'model-1',
  String title = 'Cache test',
  required List<TokenUsage> steps,
  StopReason stopReason = StopReason.endTurn,
}) async {
  final session = SessionId(sessionId);
  final turn = TurnId(turnId);
  final started = DateTime.utc(2026, 9, 1);
  final model = ModelRef(
    providerId: ProviderId(providerId),
    modelId: ModelId(modelId),
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
        id: TimelineItemId('$turnId-user'),
        sessionId: session,
        turnId: turn,
        sequence: 1,
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
          sequence: index + 2,
          occurredAt: started.add(Duration(seconds: index)),
          content: const [TextContent('ok')],
          model: model,
          stopReason: stopReason,
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
      model: model,
      completedAt: started.add(const Duration(minutes: 1)),
      usage: steps.isEmpty ? const TokenUsage() : steps.last,
    ),
  );
}

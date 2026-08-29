import 'dart:async';

import 'package:atlas_runtime/src/agent/context_compactor.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  final sessionId = SessionId('session-1');
  final turn1 = TurnId('turn-1');
  final turn2 = TurnId('turn-2');
  final turn3 = TurnId('turn-3');
  final model = ModelRef(providerId: ProviderId('test'), modelId: ModelId('m'));

  TimelineItem item(TurnId turn, int sequence, [String text = 'hello']) =>
      UserMessageItem(
        id: TimelineItemId('i$sequence'),
        sessionId: sessionId,
        turnId: turn,
        sequence: sequence,
        occurredAt: DateTime.utc(2026),
        content: [TextContent(text)],
      );

  Session session({CompactionCheckpoint? compaction}) => Session(
    id: sessionId,
    workingDirectory: '/tmp',
    createdAt: DateTime.utc(2026),
    updatedAt: DateTime.utc(2026),
    compaction: compaction,
  );

  ContextCompactor compactor(
    ModelProvider provider,
    SessionStore store, {
    double threshold = 0.8,
    int keptRecentTurns = 5,
  }) => ContextCompactor(
    provider: provider,
    store: store,
    threshold: threshold,
    keptRecentTurns: keptRecentTurns,
  );

  CompactionJob job(
    Session session,
    List<TimelineItem> timeline, {
    TokenUsage usage = const TokenUsage(),
    bool enforceThreshold = true,
    String? instruction,
    CancellationToken? cancellation,
  }) => CompactionJob(
    session: session,
    timeline: timeline,
    systemPrompt: 'system',
    model: model,
    turnId: turn2,
    latestUsage: usage,
    nextSequence: (() {
      var sequence = 0;
      return () => sequence++;
    })(),
    enforceThreshold: enforceThreshold,
    instruction: instruction,
    cancellation: cancellation,
  );

  test('skips below the threshold without touching the provider', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('s')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    final events = await compactor(provider, store)
        .compact(
          job(session(), [
            item(turn1, 1),
            item(turn1, 2),
            item(turn2, 3),
          ], usage: const TokenUsage(inputTokens: 100)),
        )
        .toList();

    expect(events, isEmpty);
    expect(provider.requests, isEmpty);
    expect(store.saved, isEmpty);
  });

  test('compacts above the threshold and keeps whole newest turns', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Summary.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    final events = await compactor(provider, store, keptRecentTurns: 2)
        .compact(
          job(session(), [
            item(turn1, 1),
            item(turn1, 2),
            item(turn2, 3),
            item(turn2, 4),
            item(turn3, 5),
          ], usage: const TokenUsage(inputTokens: 9000)),
        )
        .toList();

    expect(events.map((event) => event.runtimeType), [
      CompactionStarted,
      CompactionFinished,
    ]);
    final checkpoint = store.saved.single;
    // Kept window = newest 2 whole turns (items 3-5); boundary = item 2.
    expect(checkpoint.compactedThroughSequence, 2);
    expect(checkpoint.keptRecentMessages, 3);
    expect(checkpoint.summary, 'Summary.');
    expect(checkpoint.inputTokensBefore, 9000);
  });

  test('manual mode ignores the threshold but requires two turns', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Summary.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    final events = await compactor(provider, store)
        .compact(
          job(session(), [
            item(turn1, 1),
            item(turn1, 2),
            item(turn2, 3),
          ], enforceThreshold: false),
        )
        .toList();

    expect(events, hasLength(2));
    expect(provider.requests, hasLength(1));

    final singleTurn = await compactor(provider, store)
        .compact(
          job(session(), [
            item(turn1, 1),
            item(turn1, 2),
          ], enforceThreshold: false),
        )
        .toList();
    expect(singleTurn, isEmpty);
  });

  test('a single long turn keeps the newest item as live context', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Summary.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    await compactor(provider, store)
        .compact(
          job(session(), [
            item(turn1, 1),
            item(turn1, 2),
            item(turn1, 3),
            item(turn1, 4),
          ], usage: const TokenUsage(inputTokens: 9000)),
        )
        .toList();

    final checkpoint = store.saved.single;
    expect(checkpoint.compactedThroughSequence, 1);
    expect(checkpoint.keptRecentMessages, 4);
  });

  test('skips when the context window is unknown or describe fails', () async {
    final unknown = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('s')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final store = _RecordingStore();
    expect(
      await compactor(unknown, store)
          .compact(
            job(session(), [
              item(turn1, 1),
              item(turn2, 2),
            ], usage: const TokenUsage(inputTokens: 9000)),
          )
          .toList(),
      isEmpty,
    );

    final failing = _DescribeFailingProvider();
    expect(
      await compactor(failing, store)
          .compact(
            job(session(), [
              item(turn1, 1),
              item(turn2, 2),
            ], usage: const TokenUsage(inputTokens: 9000)),
          )
          .toList(),
      isEmpty,
    );
  });

  test('chunks oversized transcripts with a map-reduce summary', () async {
    // contextWindow 600 -> input budget 360, chunk budget 180 tokens; each
    // ~700 character item is roughly 175 tokens, so each becomes one chunk.
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('chunk 1')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('chunk 2')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('chunk 3')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('final')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 600);
    final store = _RecordingStore();
    await compactor(provider, store, keptRecentTurns: 1)
        .compact(
          job(session(), [
            item(turn1, 1, 'a' * 700),
            item(turn1, 2, 'b' * 700),
            item(turn1, 3, 'c' * 700),
            item(turn2, 4),
          ], usage: const TokenUsage(inputTokens: 5000)),
        )
        .toList();

    expect(provider.requests, hasLength(4));
    expect(
      provider.requests[0].messages.single.content,
      everyElement(
        isA<TextContent>().having(
          (part) => part.text,
          'text',
          contains('Summarize history chunk 1 of 3'),
        ),
      ),
    );
    final finalPrompt = provider.requests[3].messages.single.content;
    expect(
      finalPrompt,
      everyElement(
        isA<TextContent>().having(
          (part) => part.text,
          'text',
          allOf(contains('chunk 1'), contains('<chunk_summaries>')),
        ),
      ),
    );
    expect(store.saved.single.summary, 'final');
  });

  test('chains the previous summary and the user instruction', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('New.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    final previous = DateTime.utc(2026);
    await compactor(provider, store, keptRecentTurns: 1)
        .compact(
          job(
            session(
              compaction: CompactionCheckpoint(
                sessionId: sessionId,
                compactedThroughSequence: 0,
                summary: 'Old summary.',
                keptRecentMessages: 1,
                inputTokensBefore: 10,
                inputTokensAfter: 5,
                createdAt: previous,
              ),
            ),
            [item(turn1, 1), item(turn1, 2), item(turn2, 3)],
            usage: const TokenUsage(inputTokens: 9000),
            instruction: 'Focus on the database.',
          ),
        )
        .toList();

    final prompt = provider.requests.single.messages.single.content
        .whereType<TextContent>()
        .single
        .text;
    expect(
      prompt,
      contains('<previous_summary>\nOld summary.\n</previous_summary>'),
    );
    expect(
      prompt,
      contains('Additional user instruction:\nFocus on the database.'),
    );
  });

  test('emits CompactionFailed when the summary comes back empty', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('   ')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    final events = await compactor(provider, store)
        .compact(
          job(session(), [
            item(turn1, 1),
            item(turn1, 2),
            item(turn2, 3),
          ], usage: const TokenUsage(inputTokens: 9000)),
        )
        .toList();

    expect(events.map((event) => event.runtimeType), [
      CompactionStarted,
      CompactionFailed,
    ]);
    expect(
      (events.last as CompactionFailed).message,
      contains('Context compaction failed'),
    );
    expect(store.saved, isEmpty);
  });

  test('does nothing when already cancelled', () async {
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('s')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final store = _RecordingStore();
    final cancellation = CancellationToken()..cancel();
    final events = await compactor(provider, store)
        .compact(
          job(
            session(),
            [item(turn1, 1), item(turn2, 2)],
            usage: const TokenUsage(inputTokens: 9000),
            cancellation: cancellation,
          ),
        )
        .toList();

    expect(events, isEmpty);
    expect(provider.requests, isEmpty);
  });
}

/// Streams scripted responses and records every model request.
final class _ScriptedProvider implements ModelProvider {
  _ScriptedProvider(this.responses, {this.contextWindow = 0});

  final List<ModelResponse> responses;
  final int contextWindow;
  final requests = <ModelRequest>[];
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef described) async => ModelDescriptor(
    ref: described,
    contextWindow: contextWindow,
    inputCapabilities: const {ModelInputCapability.text},
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    yield ModelCompletedEvent(responses[_index++]);
  }
}

/// Fails every describe call.
final class _DescribeFailingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef described) async =>
      throw StateError('describe failed');

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield ModelCompletedEvent(
      const ModelResponse(
        content: [TextContent('s')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

/// Records persisted compaction checkpoints only.
final class _RecordingStore implements SessionStore {
  final saved = <CompactionCheckpoint>[];

  @override
  Future<void> saveCompaction(
    SessionId id,
    CompactionCheckpoint checkpoint,
  ) async => saved.add(checkpoint);

  @override
  Future<void> beginTurn(BeginTurn operation) async =>
      throw UnimplementedError();

  @override
  Future<void> createSession(Session session) async =>
      throw UnimplementedError();

  @override
  Future<void> deleteSession(SessionId sessionId) async =>
      throw UnimplementedError();

  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) async => throw UnimplementedError();

  @override
  Future<void> appendToolResult(
    SessionId sessionId,
    ToolResultItem item,
  ) async => throw UnimplementedError();

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) async =>
      throw UnimplementedError();

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) async =>
      throw UnimplementedError();

  @override
  Future<SessionPage> listSessions(SessionQuery query) async =>
      throw UnimplementedError();

  @override
  Future<void> renameSession(SessionId sessionId, String title) async =>
      throw UnimplementedError();
}

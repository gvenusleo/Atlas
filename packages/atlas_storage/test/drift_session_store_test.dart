import 'package:atlas_runtime/atlas_runtime.dart' as runtime;
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_storage/src/database/database.dart';
import 'package:drift/native.dart';
import 'package:test/test.dart';

void main() {
  late AtlasDatabase database;
  late DriftSessionStore store;

  setUp(() {
    database = AtlasDatabase(NativeDatabase.memory());
    store = DriftSessionStore(database);
  });

  tearDown(() => store.close());

  test('round trips typed timeline and model continuation', () async {
    final session = _session('session-1', updatedAt: DateTime.utc(2026, 1, 2));
    final turn = _turn(session.id, 'turn-1');
    final user = runtime.UserMessageItem(
      id: const runtime.TimelineItemId('item-1'),
      sessionId: session.id,
      turnId: turn.id,
      sequence: 0,
      occurredAt: DateTime.utc(2026, 1, 2),
      content: const [runtime.TextContent('hello')],
    );
    final assistant = runtime.AssistantMessageItem(
      id: const runtime.TimelineItemId('item-2'),
      sessionId: session.id,
      turnId: turn.id,
      sequence: 1,
      occurredAt: DateTime.utc(2026, 1, 2, 0, 0, 1),
      content: const [runtime.TextContent('world')],
      model: _model,
      stopReason: runtime.StopReason.endTurn,
    );
    final checkpoint = runtime.ModelCheckpoint(
      timelineItemId: assistant.id,
      continuation: const runtime.ModelContinuation(
        providerId: runtime.ProviderId('provider'),
        reasoningSummary: 'summary',
        opaquePayload: <String, Object?>{'cursor': 'next'},
      ),
      createdAt: DateTime.utc(2026, 1, 2, 0, 0, 2),
    );

    await store.beginTurn(
      runtime.BeginTurn(session: session, turn: turn, userMessage: user),
    );
    await store.appendModelStep(
      session.id,
      runtime.PersistedModelStep(
        assistantMessage: assistant,
        toolCalls: const [],
        checkpoint: checkpoint,
      ),
    );
    await store.saveCompaction(
      session.id,
      runtime.CompactionCheckpoint(
        sessionId: session.id,
        compactedThroughSequence: 1,
        summary: 'summary',
        inputTokensBefore: 10,
        inputTokensAfter: 4,
        createdAt: DateTime.utc(2026, 1, 2, 0, 0, 3),
      ),
    );

    final loaded = await store.loadSession(session.id);
    expect(loaded.timeline, hasLength(2));
    expect(loaded.timeline[0], isA<runtime.UserMessageItem>());
    expect(loaded.timeline[1], isA<runtime.AssistantMessageItem>());
    expect(
      loaded.modelCheckpoints.single.continuation.opaquePayload['cursor'],
      'next',
    );
    expect(loaded.session.compaction?.inputTokensAfter, 4);
  });

  test('lists sessions with a stable cursor and cascades deletes', () async {
    final first = _session('session-a', updatedAt: DateTime.utc(2026, 1, 1));
    final second = _session('session-b', updatedAt: DateTime.utc(2026, 1, 2));
    await store.createSession(first);
    await store.createSession(second);
    final page = await store.listSessions(const runtime.SessionQuery(limit: 1));
    expect(page.items.single.id, second.id);
    expect(page.nextCursor, isNotNull);
    final next = await store.listSessions(
      runtime.SessionQuery(cursor: page.nextCursor, limit: 1),
    );
    expect(next.items.single.id, first.id);

    final turn = _turn(first.id, 'turn-delete');
    final item = runtime.UserMessageItem(
      id: const runtime.TimelineItemId('item-delete'),
      sessionId: first.id,
      turnId: turn.id,
      sequence: 0,
      occurredAt: first.updatedAt,
      content: const [runtime.TextContent('delete me')],
    );
    await store.beginTurn(
      runtime.BeginTurn(session: first, turn: turn, userMessage: item),
    );
    await store.deleteSession(first.id);
    expect(
      () => store.loadSession(first.id),
      throwsA(isA<runtime.SessionNotFoundException>()),
    );
    expect(await (database.select(database.timelineItems)).get(), isEmpty);
    expect(await (database.select(database.turns)).get(), isEmpty);
  });

  test('rolls back an invalid checkpoint with its assistant item', () async {
    final session = _session('session-rollback', updatedAt: DateTime.utc(2026));
    final turn = _turn(session.id, 'turn-rollback');
    final user = runtime.UserMessageItem(
      id: const runtime.TimelineItemId('item-user'),
      sessionId: session.id,
      turnId: turn.id,
      sequence: 0,
      occurredAt: session.updatedAt,
      content: const [runtime.TextContent('hello')],
    );
    final assistant = runtime.AssistantMessageItem(
      id: const runtime.TimelineItemId('item-assistant'),
      sessionId: session.id,
      turnId: turn.id,
      sequence: 1,
      occurredAt: session.updatedAt,
      content: const [runtime.TextContent('world')],
      model: _model,
      stopReason: runtime.StopReason.endTurn,
    );
    await store.beginTurn(
      runtime.BeginTurn(session: session, turn: turn, userMessage: user),
    );

    await expectLater(
      store.appendModelStep(
        session.id,
        runtime.PersistedModelStep(
          assistantMessage: assistant,
          toolCalls: const [],
          checkpoint: runtime.ModelCheckpoint(
            timelineItemId: const runtime.TimelineItemId('wrong-item'),
            continuation: const runtime.ModelContinuation(
              providerId: runtime.ProviderId('provider'),
            ),
            createdAt: session.updatedAt,
          ),
        ),
      ),
      throwsFormatException,
    );

    final loaded = await store.loadSession(session.id);
    expect(loaded.timeline, hasLength(1));
    expect(loaded.modelCheckpoints, isEmpty);
  });

  test(
    'rejects timeline items whose turn belongs to another session',
    () async {
      final first = _session('session-first', updatedAt: DateTime.utc(2026));
      final second = _session('session-second', updatedAt: DateTime.utc(2026));
      final firstTurn = _turn(first.id, 'turn-first');
      final secondTurn = _turn(second.id, 'turn-second');
      await store.beginTurn(
        runtime.BeginTurn(
          session: first,
          turn: firstTurn,
          userMessage: _user(first, firstTurn, 'first'),
        ),
      );
      await store.beginTurn(
        runtime.BeginTurn(
          session: second,
          turn: secondTurn,
          userMessage: _user(second, secondTurn, 'second'),
        ),
      );

      final forged = runtime.ToolResultItem(
        id: const runtime.TimelineItemId('item-forged'),
        sessionId: first.id,
        turnId: secondTurn.id,
        sequence: 1,
        occurredAt: first.updatedAt,
        callId: const runtime.ToolCallId('call-forged'),
        content: 'forged',
      );
      await expectLater(
        store.appendToolResult(first.id, forged),
        throwsFormatException,
      );
      expect((await store.loadSession(first.id)).timeline, hasLength(1));
    },
  );

  test('does not finish a turn from another session', () async {
    final first = _session(
      'session-finish-first',
      updatedAt: DateTime.utc(2026),
    );
    final second = _session(
      'session-finish-second',
      updatedAt: DateTime.utc(2026),
    );
    final firstTurn = _turn(first.id, 'turn-finish-first');
    await store.beginTurn(
      runtime.BeginTurn(
        session: first,
        turn: firstTurn,
        userMessage: _user(first, firstTurn, 'first'),
      ),
    );

    final forged = runtime.Turn(
      id: firstTurn.id,
      sessionId: second.id,
      status: runtime.TurnStatus.completed,
      startedAt: firstTurn.startedAt,
      completedAt: DateTime.utc(2026, 1, 2),
    );
    await expectLater(
      store.finishTurn(second.id, forged),
      throwsA(isA<runtime.SessionNotFoundException>()),
    );
    expect(
      (await store.loadSession(first.id)).turns.single.status,
      runtime.TurnStatus.running,
    );
  });
}

const _model = runtime.ModelRef(
  providerId: runtime.ProviderId('provider'),
  modelId: runtime.ModelId('model'),
);

runtime.Session _session(String id, {required DateTime updatedAt}) =>
    runtime.Session(
      id: runtime.SessionId(id),
      workingDirectory: '/tmp',
      createdAt: updatedAt,
      updatedAt: updatedAt,
    );

runtime.Turn _turn(runtime.SessionId sessionId, String id) => runtime.Turn(
  id: runtime.TurnId(id),
  sessionId: sessionId,
  status: runtime.TurnStatus.running,
  startedAt: DateTime.utc(2026, 1, 2),
  model: _model,
);

runtime.UserMessageItem _user(
  runtime.Session session,
  runtime.Turn turn,
  String text,
) => runtime.UserMessageItem(
  id: runtime.TimelineItemId('item-${turn.id.value}'),
  sessionId: session.id,
  turnId: turn.id,
  sequence: 0,
  occurredAt: session.updatedAt,
  content: [runtime.TextContent(text)],
);

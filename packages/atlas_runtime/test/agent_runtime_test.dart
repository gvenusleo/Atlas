import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  test('persists and emits a tool loop in occurrence order', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
      ModelResponse(
        content: const [TextContent('I will inspect the files.')],
        toolCalls: const [
          ToolCall(
            id: ToolCallId('call-1'),
            name: 'inspect',
            arguments: <String, Object?>{'path': '.'},
          ),
        ],
        stopReason: StopReason.toolUse,
        continuation: ModelContinuation(
          providerId: ProviderId('test'),
          reasoningSummary: 'inspect first',
          opaquePayload: <String, Object?>{'cursor': 'one'},
        ),
      ),
      const ModelResponse(
        content: [TextContent('Done.')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final toolRegistry = _MemoryTools(
      result: const ToolResult(content: 'file list', isError: false),
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: toolRegistry,
      ids: _Ids(),
      defaultModel: const ModelRef(
        providerId: ProviderId('test'),
        modelId: ModelId('model'),
      ),
    );

    final events = await runtime
        .run(
          const TurnRequest(
            content: [TextContent('Inspect the files')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    expect(events.map((event) => event.runtimeType), [
      TurnStarted,
      ModelTextDelta,
      ModelResponseReceived,
      ToolStarted,
      ToolFinished,
      ModelTextDelta,
      ModelResponseReceived,
      TurnFinished,
    ]);
    expect(provider.requests, hasLength(2));
    final secondMessages = provider.requests[1].messages;
    final assistant = secondMessages.firstWhere(
      (message) => message.role == ModelMessageRole.assistant,
    );
    expect(assistant.continuation?.opaquePayload['cursor'], 'one');
    expect(secondMessages.last.toolOutput, 'file list');
    expect(store.timeline, hasLength(5));
    expect(store.checkpoints, hasLength(1));
    expect(store.turns.single.status, TurnStatus.completed);
  });

  test('writes a paired error result when a tool throws', () async {
    final store = _MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: _ScriptedProvider([
        const ModelResponse(
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'inspect',
              arguments: <String, Object?>{},
            ),
          ],
          stopReason: StopReason.toolUse,
        ),
      ]),
      tools: _ThrowingTools(),
      ids: _Ids(),
      defaultModel: const ModelRef(
        providerId: ProviderId('test'),
        modelId: ModelId('model'),
      ),
      maxSteps: 1,
    );

    final events = <AgentEvent>[];
    try {
      await for (final event in runtime.run(
        const TurnRequest(
          content: [TextContent('Inspect')],
          workingDirectory: '/tmp',
        ),
      )) {
        events.add(event);
      }
      fail('expected maximum-step failure');
    } on StateError catch (error) {
      expect(error.message, contains('maximum model steps exceeded'));
    }

    final finished = events.whereType<ToolFinished>().single;
    expect(finished.result.isError, isTrue);
    expect(finished.result.content, contains('StateError'));
    expect(store.timeline.whereType<ToolResultItem>(), hasLength(1));
    expect(store.turns.single.status, TurnStatus.failed);
  });

  test('persists user input before a provider failure', () async {
    final store = _MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: _FailingProvider(),
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
    );
    final events = <AgentEvent>[];

    await expectLater(() async {
      await for (final event in runtime.run(
        const TurnRequest(
          content: [TextContent('keep this')],
          workingDirectory: '/tmp',
        ),
      )) {
        events.add(event);
      }
    }(), throwsStateError);

    expect(store.timeline.whereType<UserMessageItem>(), hasLength(1));
    expect(store.turns.single.status, TurnStatus.failed);
    expect(events.last, isA<TurnFinished>());
  });

  test('cancellation before start does not create history', () async {
    final cancellation = CancellationToken()..cancel();
    final store = _MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: _ScriptedProvider(const []),
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
    );

    await expectLater(
      runtime
          .run(
            TurnRequest(
              content: const [TextContent('cancel')],
              workingDirectory: '/tmp',
              cancellation: cancellation,
            ),
          )
          .toList(),
      throwsA(isA<TurnCancelledException>()),
    );
    expect(store.session, isNull);
    expect(store.timeline, isEmpty);
  });

  test('cancellation after begin preserves a cancelled turn', () async {
    final store = _MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: _CancellingProvider(),
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
    );

    final events = await runtime
        .run(
          const TurnRequest(
            content: [TextContent('cancel later')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    expect(store.timeline.whereType<UserMessageItem>(), hasLength(1));
    expect(store.turns.single.status, TurnStatus.cancelled);
    expect(events.last, isA<TurnFinished>());
    expect((events.last as TurnFinished).outcome.status, TurnStatus.cancelled);
  });

  test(
    'projects the active compaction summary into the next request',
    () async {
      final store = _MemorySessionStore();
      final sessionTime = DateTime.utc(2026, 1, 1);
      final session = Session(
        id: const SessionId('compacted-session'),
        workingDirectory: '/tmp',
        createdAt: sessionTime,
        updatedAt: sessionTime,
        compaction: CompactionCheckpoint(
          sessionId: const SessionId('compacted-session'),
          compactedThroughSequence: 1,
          summary: 'The earlier task decided to use Drift.',
          inputTokensBefore: 100,
          inputTokensAfter: 20,
          createdAt: sessionTime,
        ),
      );
      store.session = session;
      final previousTurn = Turn(
        id: const TurnId('previous-turn'),
        sessionId: session.id,
        status: TurnStatus.completed,
        startedAt: sessionTime,
        completedAt: sessionTime,
        model: _model,
      );
      store.turns.add(previousTurn);
      store.timeline.addAll([
        UserMessageItem(
          id: const TimelineItemId('old-user'),
          sessionId: session.id,
          turnId: previousTurn.id,
          sequence: 0,
          occurredAt: sessionTime,
          content: const [TextContent('old request')],
        ),
        AssistantMessageItem(
          id: const TimelineItemId('old-assistant'),
          sessionId: session.id,
          turnId: previousTurn.id,
          sequence: 1,
          occurredAt: sessionTime,
          content: const [TextContent('old response')],
          model: _model,
          stopReason: StopReason.endTurn,
        ),
      ]);
      final provider = _ScriptedProvider([
        const ModelResponse(
          content: [TextContent('new response')],
          stopReason: StopReason.endTurn,
        ),
      ]);
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: _MemoryTools(result: const ToolResult(content: 'unused')),
        ids: _Ids(),
        defaultModel: _model,
        systemPromptBuilder: (_, _) => 'base prompt',
      );

      await runtime
          .run(
            const TurnRequest(
              sessionId: SessionId('compacted-session'),
              content: [TextContent('new request')],
            ),
          )
          .toList();

      expect(provider.requests.single.systemPrompt, contains('Drift'));
      expect(provider.requests.single.messages, hasLength(1));
      expect(
        textFromContent(provider.requests.single.messages.single.content),
        'new request',
      );
    },
  );

  test('serializes concurrent turns for one session', () async {
    final store = _MemorySessionStore();
    final sessionTime = DateTime.utc(2026, 1, 1);
    store.session = Session(
      id: const SessionId('shared-session'),
      workingDirectory: '/tmp',
      createdAt: sessionTime,
      updatedAt: sessionTime,
    );
    final provider = _BlockingProvider();
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
    );

    final first = runtime
        .run(
          const TurnRequest(
            sessionId: SessionId('shared-session'),
            content: [TextContent('first')],
          ),
        )
        .toList();
    await provider.firstRequestStarted.future;
    final second = runtime
        .run(
          const TurnRequest(
            sessionId: SessionId('shared-session'),
            content: [TextContent('second')],
          ),
        )
        .toList();
    await Future<void>.delayed(Duration.zero);
    expect(provider.requests, hasLength(1));
    provider.releaseFirst.complete();
    await Future.wait([first, second]);

    expect(
      store.timeline.whereType<UserMessageItem>().map((item) => item.sequence),
      [0, 2],
    );
  });
}

const _model = ModelRef(
  providerId: ProviderId('test'),
  modelId: ModelId('model'),
);

final class _Ids implements IdGenerator {
  var _session = 0;
  var _turn = 0;
  var _item = 0;

  @override
  SessionId sessionId() => SessionId('session-${++_session}');

  @override
  TurnId turnId() => TurnId('turn-${++_turn}');

  @override
  TimelineItemId timelineItemId() => TimelineItemId('item-${++_item}');
}

final class _ScriptedProvider implements ModelProvider {
  _ScriptedProvider(this.responses);

  final List<ModelResponse> responses;
  final requests = <ModelRequest>[];
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    yield const TextDeltaEvent('delta');
    yield ModelCompletedEvent(responses[_index++]);
  }
}

final class _FailingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield ModelFailedEvent(StateError('provider failed'), StackTrace.current);
  }
}

final class _BlockingProvider implements ModelProvider {
  final firstRequestStarted = Completer<void>();
  final releaseFirst = Completer<void>();
  final requests = <ModelRequest>[];

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    if (requests.length == 1) {
      firstRequestStarted.complete();
      await releaseFirst.future;
    }
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('done')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class _CancellingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    request.cancellation!.cancel();
    yield const TextDeltaEvent('ignored');
  }
}

final class _MemoryTools implements ToolRegistry {
  _MemoryTools({required this.result});

  final ToolResult result;

  @override
  List<ToolDescriptor> get descriptors => const [
    ToolDescriptor(
      name: 'inspect',
      description: 'Inspect files',
      inputSchema: <String, Object?>{},
    ),
  ];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async =>
      result;
}

final class _ThrowingTools implements ToolRegistry {
  @override
  List<ToolDescriptor> get descriptors => const [];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async {
    throw StateError('tool failed');
  }
}

final class _MemorySessionStore implements SessionStore {
  Session? session;
  final turns = <Turn>[];
  final timeline = <TimelineItem>[];
  final checkpoints = <ModelCheckpoint>[];
  CompactionCheckpoint? compaction;

  @override
  Future<void> createSession(Session value) async => session = value;

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) async {
    final value = session;
    if (value == null || value.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    return SessionSnapshot(
      session: value,
      turns: List.unmodifiable(turns),
      timeline: List.unmodifiable(timeline),
      modelCheckpoints: List.unmodifiable(checkpoints),
    );
  }

  @override
  Future<SessionPage> listSessions(SessionQuery query) async => SessionPage(
    items: [
      if (session != null)
        SessionSummary(
          id: session!.id,
          title: session!.title,
          workingDirectory: session!.workingDirectory,
          updatedAt: session!.updatedAt,
        ),
    ],
  );

  @override
  Future<void> beginTurn(BeginTurn operation) async {
    if (timeline.any(
      (item) => item.sequence == operation.userMessage.sequence,
    )) {
      throw StateError('duplicate timeline sequence');
    }
    session = operation.session;
    turns.add(operation.turn);
    timeline.add(operation.userMessage);
  }

  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) async {
    timeline.add(operation.assistantMessage);
    timeline.addAll(operation.toolCalls);
    if (operation.checkpoint != null) checkpoints.add(operation.checkpoint!);
  }

  @override
  Future<void> appendToolResult(
    SessionId sessionId,
    ToolResultItem item,
  ) async => timeline.add(item);

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) async {
    turns[turns.indexWhere((item) => item.id == turn.id)] = turn;
  }

  @override
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  ) async => compaction = checkpoint;

  @override
  Future<void> deleteSession(SessionId sessionId) async => session = null;
}

import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'test_fakes.dart';

void main() {
  test('createSession persists a blank session that can be loaded', () async {
    final store = MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: ScriptedProvider(const []),
      tools: ThrowingTools(),
      ids: TestIds(),
      defaultModel: testModel,
    );

    final session = await runtime.createSession(
      workingDirectory: '/tmp',
      additionalDirectories: ['/shared'],
    );

    expect(session.workingDirectory, '/tmp');
    expect(session.additionalDirectories, ['/shared']);
    expect(session.compaction, isNull);
    final snapshot = await runtime.loadSession(session.id);
    expect(snapshot.session.id, session.id);
    expect(snapshot.timeline, isEmpty);
    expect(snapshot.turns, isEmpty);
  });
  test('createSession rejects an empty working directory', () async {
    final runtime = AgentRuntime(
      store: MemorySessionStore(),
      provider: ScriptedProvider(const []),
      tools: ThrowingTools(),
      ids: TestIds(),
      defaultModel: testModel,
    );

    await expectLater(
      runtime.createSession(workingDirectory: ''),
      throwsArgumentError,
    );
  });
  test('persists and emits a tool loop in occurrence order', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider([
      ModelResponse(
        content: const [TextContent('I will inspect the files.')],
        toolCalls: [
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
    final toolRegistry = MemoryTools(
      result: const ToolResult(content: 'file list', isError: false),
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: toolRegistry,
      ids: TestIds(),
      defaultModel: testModel,
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
    final store = MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: ScriptedProvider([
        ModelResponse(
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
      tools: ThrowingTools(),
      ids: TestIds(),
      defaultModel: testModel,
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
  test('drops orphan tool calls when resuming a session', () async {
    final store = MemorySessionStore();
    final sessionId = SessionId('session-1');
    final turnId = TurnId('turn-1');
    final now = DateTime.now().toUtc();
    store.session = Session(
      id: sessionId,
      workingDirectory: '/tmp',
      createdAt: now,
      updatedAt: now,
    );
    store.turns.add(
      Turn(
        id: turnId,
        sessionId: sessionId,
        status: TurnStatus.completed,
        startedAt: now,
        completedAt: now,
        model: testModel,
      ),
    );
    // A cancelled turn left a tool call without its result in the timeline.
    store.timeline.addAll([
      UserMessageItem(
        id: TimelineItemId('item-1'),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 1,
        occurredAt: now,
        content: const [TextContent('hello')],
      ),
      AssistantMessageItem(
        id: TimelineItemId('item-2'),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 2,
        occurredAt: now,
        content: const [],
        model: testModel,
        stopReason: StopReason.toolUse,
      ),
      ToolCallItem(
        id: TimelineItemId('item-3'),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 3,
        occurredAt: now,
        call: ToolCall(
          id: ToolCallId('call-orphan'),
          name: 'inspect',
          arguments: const <String, Object?>{},
        ),
      ),
    ]);

    final provider = ScriptedProvider(const [
      ModelResponse(
        content: [TextContent('done')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'ok')),
      ids: TestIds(),
      defaultModel: testModel,
    );

    await runtime
        .run(
          TurnRequest(
            content: const [TextContent('continue')],
            sessionId: sessionId,
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    final messages = provider.requests.single.messages;
    final orphan = messages.where(
      (message) =>
          message.role == ModelMessageRole.assistant &&
          message.toolCalls.isNotEmpty,
    );
    expect(orphan, isEmpty);
  });
  test('pairs every persisted tool call when cancellation arrives', () async {
    final store = MemorySessionStore();
    final tools = CancellingTools();
    final runtime = AgentRuntime(
      store: store,
      provider: ScriptedProvider([
        ModelResponse(
          toolCalls: [
            for (final id in ['call-1', 'call-2'])
              ToolCall(
                id: ToolCallId(id),
                name: 'inspect',
                arguments: const <String, Object?>{},
              ),
          ],
          stopReason: StopReason.toolUse,
        ),
      ]),
      tools: tools,
      ids: TestIds(),
      defaultModel: testModel,
    );

    final events = await runtime
        .run(
          const TurnRequest(
            content: [TextContent('Inspect both')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    expect(tools.calls.map((call) => call.id.value), ['call-1']);
    final results = store.timeline.whereType<ToolResultItem>().toList();
    expect(results.map((result) => result.callId.value), ['call-1', 'call-2']);
    expect(results.last.isError, isTrue);
    expect(results.last.content, 'Tool execution cancelled');
    expect((events.last as TurnFinished).outcome.status, TurnStatus.cancelled);
  });
  test('persists user input before a provider failure', () async {
    final store = MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: FailingProvider(),
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
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
    final store = MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: ScriptedProvider(const []),
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
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
    final store = MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: CancellingProvider(),
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
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
    'resumes an existing session with its original working directory',
    () async {
      final store = MemorySessionStore();
      final sessionTime = DateTime.utc(2026, 1, 1);
      store.session = Session(
        id: SessionId('existing-session'),
        workingDirectory: '/original',
        createdAt: sessionTime,
        updatedAt: sessionTime,
      );
      final provider = ScriptedProvider([
        ModelResponse(
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'inspect',
              arguments: const <String, Object?>{},
            ),
          ],
          stopReason: StopReason.toolUse,
        ),
        const ModelResponse(
          content: [TextContent('resumed')],
          stopReason: StopReason.endTurn,
        ),
      ]);
      final tools = MemoryTools(result: const ToolResult(content: 'inspected'));
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: tools,
        ids: TestIds(),
        defaultModel: testModel,
      );

      await runtime
          .run(
            TurnRequest(
              sessionId: SessionId('existing-session'),
              content: const [TextContent('resume')],
              workingDirectory: '/override',
            ),
          )
          .toList();

      expect(store.session!.workingDirectory, '/original');
      expect(tools.contexts.single.workingDirectory, '/original');
    },
  );
  test('serializes concurrent turns for one session', () async {
    final store = MemorySessionStore();
    final sessionTime = DateTime.utc(2026, 1, 1);
    store.session = Session(
      id: SessionId('shared-session'),
      workingDirectory: '/tmp',
      createdAt: sessionTime,
      updatedAt: sessionTime,
    );
    final provider = BlockingProvider();
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
    );

    final first = runtime
        .run(
          TurnRequest(
            sessionId: SessionId('shared-session'),
            content: [TextContent('first')],
          ),
        )
        .toList();
    await provider.firstRequestStarted.future;
    final second = runtime
        .run(
          TurnRequest(
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

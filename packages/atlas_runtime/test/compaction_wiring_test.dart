import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_runtime/src/agent/context_compactor.dart'
    show maxSummaryTokens;
import 'package:test/test.dart';

import 'test_fakes.dart';

void main() {
  test(
    'projects the active compaction summary into the next request',
    () async {
      final store = MemorySessionStore();
      final sessionTime = DateTime.utc(2026, 1, 1);
      final session = Session(
        id: SessionId('compacted-session'),
        workingDirectory: '/tmp',
        createdAt: sessionTime,
        updatedAt: sessionTime,
        compaction: CompactionCheckpoint(
          sessionId: SessionId('compacted-session'),
          compactedThroughSequence: 1,
          summary: 'The earlier task decided to use Drift.',
          keptRecentMessages: 2,
          inputTokensBefore: 100,
          inputTokensAfter: 20,
          createdAt: sessionTime,
        ),
      );
      store.session = session;
      final provider = ScriptedProvider([
        const ModelResponse(
          content: [TextContent('new response')],
          stopReason: StopReason.endTurn,
        ),
      ]);
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: MemoryTools(result: const ToolResult(content: 'unused')),
        ids: TestIds(),
        defaultModel: testModel,
        systemPromptBuilder: (_) => 'base prompt',
      );

      await runtime
          .run(
            TurnRequest(
              sessionId: SessionId('compacted-session'),
              content: [TextContent('new request')],
            ),
          )
          .toList();

      expect(provider.requests.single.systemPrompt, contains('Drift'));
      expect(
        provider.requests.single.systemPrompt,
        contains('Context compacted. Kept 2 recent messages.'),
      );
      expect(provider.requests.single.messages, hasLength(1));
      expect(
        textFromContent(provider.requests.single.messages.single.content),
        'new request',
      );
      expect(store.timeline.first.sequence, 2);
    },
  );
  test('compacts an exhausted context and keeps the newest turn', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider([
      const ModelResponse(
        content: [TextContent('first reply')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('second reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 9500, totalTokens: 9500),
      ),
      const ModelResponse(
        content: [TextContent('Summary of the first turn.')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('third reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 100, totalTokens: 100),
      ),
    ], contextWindow: 10000);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
      keptRecentTurns: 1,
    );

    final first = await runtime
        .run(
          const TurnRequest(
            content: [TextContent('first request')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();
    final sessionId = first.first.sessionId;
    final second = await runtime
        .run(
          TurnRequest(
            sessionId: sessionId,
            content: [TextContent('second request')],
          ),
        )
        .toList();

    expect(second.map((event) => event.runtimeType), [
      TurnStarted,
      ModelTextDelta,
      ModelResponseReceived,
      TurnFinished,
      CompactionStarted,
      CompactionFinished,
    ]);
    final checkpoint = second.whereType<CompactionFinished>().single.checkpoint;
    expect(checkpoint.summary, 'Summary of the first turn.');
    expect(checkpoint.compactedThroughSequence, 1);
    expect(checkpoint.keptRecentMessages, 2);
    expect(checkpoint.inputTokensBefore, 9500);
    expect(checkpoint.inputTokensAfter, greaterThanOrEqualTo(0));
    expect(store.compaction, same(checkpoint));

    final summaryRequest = provider.requests.last;
    expect(summaryRequest.messages.single.role, ModelMessageRole.user);
    final summaryPrompt = textFromContent(
      summaryRequest.messages.single.content,
    );
    expect(summaryPrompt, contains('<transcript>'));
    expect(summaryPrompt, contains('first reply'));
    expect(summaryPrompt, isNot(contains('second reply')));
    expect(summaryRequest.maxOutputTokens, maxSummaryTokens);

    await runtime
        .run(
          TurnRequest(
            sessionId: sessionId,
            content: [TextContent('third request')],
          ),
        )
        .toList();
    expect(
      provider.requests.last.systemPrompt,
      contains('Context compacted. Kept 2 recent messages.'),
    );
    expect(
      provider.requests.last.systemPrompt,
      contains('Summary of the first turn.'),
    );
  });
  test('manually compacts without the threshold check', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider([
      const ModelResponse(
        content: [TextContent('first reply')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('second reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 100, totalTokens: 100),
      ),
      const ModelResponse(
        content: [TextContent('Manual summary.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
      keptRecentTurns: 1,
    );

    final first = await runtime
        .run(
          const TurnRequest(
            content: [TextContent('first request')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();
    final second = await runtime
        .run(
          TurnRequest(
            sessionId: first.first.sessionId,
            content: [TextContent('second request')],
          ),
        )
        .toList();
    expect(second.whereType<CompactionStarted>(), isEmpty);

    final events = await runtime
        .compact(first.first.sessionId, instruction: 'keep files')
        .toList();
    expect(events.map((event) => event.runtimeType), [
      CompactionStarted,
      CompactionFinished,
    ]);
    final checkpoint = events.whereType<CompactionFinished>().single.checkpoint;
    expect(checkpoint.summary, 'Manual summary.');
    expect(checkpoint.compactedThroughSequence, 1);
    expect(checkpoint.keptRecentMessages, 2);
    final summaryPrompt = textFromContent(
      provider.requests.last.messages.single.content,
    );
    expect(summaryPrompt, contains('<transcript>'));
    expect(summaryPrompt, contains('Additional user instruction:\nkeep files'));
  });
  test(
    'manual compaction skips when nothing is outside the kept window',
    () async {
      final store = MemorySessionStore();
      final provider = ScriptedProvider([
        const ModelResponse(
          content: [TextContent('first reply')],
          stopReason: StopReason.endTurn,
        ),
      ]);
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: MemoryTools(result: const ToolResult(content: 'unused')),
        ids: TestIds(),
        defaultModel: testModel,
        keptRecentTurns: 1,
      );

      final first = await runtime
          .run(
            const TurnRequest(
              content: [TextContent('first request')],
              workingDirectory: '/tmp',
            ),
          )
          .toList();
      final events = await runtime.compact(first.first.sessionId).toList();

      expect(events, isEmpty);
      expect(provider.requests, hasLength(1));
      expect(store.compaction, isNull);
    },
  );
  test('emits CompactionFailed without failing the turn', () async {
    final store = MemorySessionStore();
    final provider = SummaryFailingProvider([
      const ModelResponse(
        content: [TextContent('first reply')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('second reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 9500, totalTokens: 9500),
      ),
    ]);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
      keptRecentTurns: 1,
    );

    final first = await runtime
        .run(
          const TurnRequest(
            content: [TextContent('first request')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();
    final second = await runtime
        .run(
          TurnRequest(
            sessionId: first.first.sessionId,
            content: [TextContent('second request')],
          ),
        )
        .toList();

    expect(second.last, isA<CompactionFailed>());
    expect(
      second.whereType<TurnFinished>().single.outcome.status,
      TurnStatus.completed,
    );
    expect(store.compaction, isNull);
  });
  test('attempts compaction after a failed turn', () async {
    final store = MemorySessionStore();
    final provider = FirstOkThenFailProvider(
      const ModelResponse(
        content: [TextContent('first reply')],
        stopReason: StopReason.endTurn,
      ),
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
      keptRecentTurns: 1,
    );

    final first = await runtime
        .run(
          TurnRequest(
            content: [TextContent('first request')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();
    Object? thrown;
    final events = <AgentEvent>[];
    try {
      await for (final event in runtime.run(
        TurnRequest(
          sessionId: first.first.sessionId,
          content: [TextContent('x' * 400)],
        ),
      )) {
        events.add(event);
      }
    } catch (error) {
      thrown = error;
    }

    expect(thrown, isA<StateError>());
    expect(events.map((event) => event.runtimeType), [
      TurnStarted,
      TurnFinished,
      CompactionStarted,
      CompactionFailed,
    ]);
    expect(
      events.whereType<TurnFinished>().single.outcome.status,
      TurnStatus.failed,
    );
  });
}

import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  test('rejects an empty identifier', () {
    expect(() => SessionId(''), throwsArgumentError);
  });

  test('createSession persists a blank session that can be loaded', () async {
    final store = _MemorySessionStore();
    final runtime = AgentRuntime(
      store: store,
      provider: _ScriptedProvider(const []),
      tools: _ThrowingTools(),
      ids: _Ids(),
      defaultModel: _model,
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
      store: _MemorySessionStore(),
      provider: _ScriptedProvider(const []),
      tools: _ThrowingTools(),
      ids: _Ids(),
      defaultModel: _model,
    );

    await expectLater(
      runtime.createSession(workingDirectory: ''),
      throwsArgumentError,
    );
  });

  test('persists and emits a tool loop in occurrence order', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
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
    final toolRegistry = _MemoryTools(
      result: const ToolResult(content: 'file list', isError: false),
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: toolRegistry,
      ids: _Ids(),
      defaultModel: _model,
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
      tools: _ThrowingTools(),
      ids: _Ids(),
      defaultModel: _model,
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

  test('injects selected skills as leading non-persistent messages', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Done.')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
      sessionContextBuilder: _contextBuilder(
        _MemorySkillCatalog(const [
          Skill(
            name: 'alpha',
            description: 'Alpha skill.',
            dir: '/skills/alpha',
            path: '/skills/alpha/SKILL.md',
            content: '# Alpha\n\nFollow these steps.',
          ),
        ]),
      ),
    );

    await runtime
        .run(
          const TurnRequest(
            content: [TextContent('Use alpha')],
            workingDirectory: '/tmp',
            skills: ['alpha', 'missing', 'alpha'],
          ),
        )
        .toList();

    final messages = provider.requests.single.messages;
    expect(messages.first.role, ModelMessageRole.user);
    expect(
      textFromContent(messages.first.content),
      contains(
        '<skill>\n<name>alpha</name>\n<path>/skills/alpha/SKILL.md</path>',
      ),
    );
    expect(textFromContent(messages.first.content), contains('Follow these'));
    expect(messages.last.role, ModelMessageRole.user);
    expect(textFromContent(messages.last.content), 'Use alpha');
    expect(store.timeline, hasLength(2));
    expect(store.timeline.whereType<UserMessageItem>().single.content, const [
      TextContent('Use alpha'),
    ]);
    expect(
      store.timeline.whereType<AssistantMessageItem>().single.content,
      const [TextContent('Done.')],
    );
  });

  test('skips unknown and disabled skills when injecting', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Done.')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
      sessionContextBuilder: _contextBuilder(
        _MemorySkillCatalog(const [
          Skill(
            name: 'hidden',
            description: 'Hidden.',
            dir: '/skills/hidden',
            path: '/skills/hidden/SKILL.md',
            content: '# Hidden',
            disableModelInvocation: true,
          ),
        ]),
      ),
    );

    await runtime
        .run(
          const TurnRequest(
            content: [TextContent('Ask')],
            workingDirectory: '/tmp',
            skills: ['missing', 'hidden'],
          ),
        )
        .toList();

    expect(provider.requests.single.messages, hasLength(1));
    expect(
      textFromContent(provider.requests.single.messages.single.content),
      'Ask',
    );
  });

  test(
    'fails the turn when selected skill instructions are too large',
    () async {
      final store = _MemorySessionStore();
      final runtime = AgentRuntime(
        store: store,
        provider: _ScriptedProvider(const []),
        tools: _MemoryTools(result: const ToolResult(content: 'unused')),
        ids: _Ids(),
        defaultModel: _model,
        sessionContextBuilder: _contextBuilder(
          _MemorySkillCatalog([
            Skill(
              name: 'huge',
              description: 'Huge.',
              dir: '/skills/huge',
              path: '/skills/huge/SKILL.md',
              content: 'x' * (AgentRuntime.maxSelectedSkillBytes + 1),
            ),
          ]),
        ),
      );

      final events = <AgentEvent>[];
      await expectLater(() async {
        await for (final event in runtime.run(
          const TurnRequest(
            content: [TextContent('Use huge')],
            workingDirectory: '/tmp',
            skills: ['huge'],
          ),
        )) {
          events.add(event);
        }
      }(), throwsStateError);

      expect(store.turns.single.status, TurnStatus.failed);
      expect(events.last, isA<TurnFinished>());
      expect(
        (events.last as TurnFinished).outcome.failure?.code,
        'turn_failed',
      );
    },
  );

  test(
    'resumes an existing session with its original working directory',
    () async {
      final store = _MemorySessionStore();
      final sessionTime = DateTime.utc(2026, 1, 1);
      store.session = Session(
        id: SessionId('existing-session'),
        workingDirectory: '/original',
        createdAt: sessionTime,
        updatedAt: sessionTime,
      );
      final provider = _ScriptedProvider([
        const ModelResponse(
          content: [TextContent('resumed')],
          stopReason: StopReason.endTurn,
        ),
      ]);
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: _MemoryTools(result: const ToolResult(content: 'unused')),
        ids: _Ids(),
        defaultModel: _model,
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
    },
  );

  test('serializes concurrent turns for one session', () async {
    final store = _MemorySessionStore();
    final sessionTime = DateTime.utc(2026, 1, 1);
    store.session = Session(
      id: SessionId('shared-session'),
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

  test('estimates tokens from text length', () {
    expect(AgentRuntime.estimateTokens(''), 0);
    expect(AgentRuntime.estimateTokens('abcd'), 1);
    expect(AgentRuntime.estimateTokens('a' * 100), 25);
  });

  test('compacts an exhausted context and keeps the newest turn', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
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
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
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
    expect(summaryRequest.maxOutputTokens, AgentRuntime.maxSummaryTokens);

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

  test('skips compaction below the context threshold', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
      const ModelResponse(
        content: [TextContent('first reply')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('second reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 100, totalTokens: 100),
      ),
    ], contextWindow: 10000);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
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
    expect(second.whereType<CompactionFinished>(), isEmpty);
    expect(second.whereType<CompactionFailed>(), isEmpty);
    expect(store.compaction, isNull);
  });

  test('manually compacts without the threshold check', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
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
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
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
      final store = _MemorySessionStore();
      final provider = _ScriptedProvider([
        const ModelResponse(
          content: [TextContent('first reply')],
          stopReason: StopReason.endTurn,
        ),
      ]);
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: _MemoryTools(result: const ToolResult(content: 'unused')),
        ids: _Ids(),
        defaultModel: _model,
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

  test('skips compaction when describe fails', () async {
    final store = _MemorySessionStore();
    final provider = _DescribeFailingProvider([
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
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
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
    expect(second.whereType<CompactionFinished>(), isEmpty);
    expect(second.whereType<CompactionFailed>(), isEmpty);
    expect(store.compaction, isNull);
  });

  test('chains the previous summary into compaction', () async {
    final store = _MemorySessionStore();
    final sessionTime = DateTime.utc(2026, 1, 1);
    store.session = Session(
      id: SessionId('chained-session'),
      workingDirectory: '/tmp',
      createdAt: sessionTime,
      updatedAt: sessionTime,
      compaction: CompactionCheckpoint(
        sessionId: SessionId('chained-session'),
        compactedThroughSequence: -1,
        summary: 'Old summary.',
        keptRecentMessages: 0,
        inputTokensBefore: 0,
        inputTokensAfter: 0,
        createdAt: sessionTime,
      ),
    );
    final provider = _ScriptedProvider([
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
        content: [TextContent('Chained summary.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
      keptRecentTurns: 1,
    );

    await runtime
        .run(
          TurnRequest(
            sessionId: SessionId('chained-session'),
            content: [TextContent('first request')],
          ),
        )
        .toList();
    await runtime
        .run(
          TurnRequest(
            sessionId: SessionId('chained-session'),
            content: [TextContent('second request')],
          ),
        )
        .toList();

    final summaryPrompt = textFromContent(
      provider.requests.last.messages.single.content,
    );
    expect(summaryPrompt, contains('<previous_summary>'));
    expect(summaryPrompt, contains('Old summary.'));
    expect(store.compaction!.summary, 'Chained summary.');
  });

  test('compacts again with a chained and shrinking transcript', () async {
    final store = _MemorySessionStore();
    final sessionTime = DateTime.utc(2026, 1, 1);
    store.session = Session(
      id: SessionId('double-session'),
      workingDirectory: '/tmp',
      createdAt: sessionTime,
      updatedAt: sessionTime,
      compaction: CompactionCheckpoint(
        sessionId: SessionId('double-session'),
        compactedThroughSequence: -1,
        summary: 'Old summary.',
        keptRecentMessages: 0,
        inputTokensBefore: 0,
        inputTokensAfter: 0,
        createdAt: sessionTime,
      ),
    );
    final provider = _ScriptedProvider([
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
        content: [TextContent('First summary.')],
        stopReason: StopReason.endTurn,
      ),
      const ModelResponse(
        content: [TextContent('third reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 100, totalTokens: 100),
      ),
      const ModelResponse(
        content: [TextContent('fourth reply')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: 9500, totalTokens: 9500),
      ),
      const ModelResponse(
        content: [TextContent('Second summary.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
      keptRecentTurns: 1,
    );

    for (final text in ['first request', 'second request', 'third request']) {
      await runtime
          .run(
            TurnRequest(
              sessionId: SessionId('double-session'),
              content: [TextContent(text)],
            ),
          )
          .toList();
    }
    final firstCheckpoint = store.compaction!;
    expect(firstCheckpoint.summary, 'First summary.');
    expect(firstCheckpoint.compactedThroughSequence, 1);

    final fourth = await runtime
        .run(
          TurnRequest(
            sessionId: SessionId('double-session'),
            content: [TextContent('fourth request')],
          ),
        )
        .toList();
    expect(fourth.whereType<CompactionFinished>(), hasLength(1));

    final secondCheckpoint = store.compaction!;
    expect(secondCheckpoint.summary, 'Second summary.');
    expect(
      secondCheckpoint.compactedThroughSequence,
      greaterThan(firstCheckpoint.compactedThroughSequence),
    );
    final summaryPrompt = textFromContent(
      provider.requests.last.messages.single.content,
    );
    expect(summaryPrompt, contains('<previous_summary>'));
    expect(summaryPrompt, contains('First summary.'));
    expect(summaryPrompt, contains('second reply'));
    expect(summaryPrompt, contains('third reply'));
    expect(summaryPrompt, isNot(contains('first reply')));
  });

  test('emits CompactionFailed without failing the turn', () async {
    final store = _MemorySessionStore();
    final provider = _SummaryFailingProvider([
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
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
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
    final store = _MemorySessionStore();
    final provider = _FirstOkThenFailProvider(
      const ModelResponse(
        content: [TextContent('first reply')],
        stopReason: StopReason.endTurn,
      ),
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: _MemoryTools(result: const ToolResult(content: 'unused')),
      ids: _Ids(),
      defaultModel: _model,
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

final _model = ModelRef(
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
  _ScriptedProvider(this.responses, {this.contextWindow = 0});

  final List<ModelResponse> responses;
  final int contextWindow;
  final requests = <ModelRequest>[];
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: contextWindow);

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

/// Succeeds for the scripted responses, then fails further model calls.
final class _SummaryFailingProvider implements ModelProvider {
  _SummaryFailingProvider(this.responses);

  final List<ModelResponse> responses;
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: 10000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (_index < responses.length) {
      yield ModelCompletedEvent(responses[_index++]);
      return;
    }
    yield ModelFailedEvent(StateError('summary failed'), StackTrace.current);
  }
}

/// Fails every describe call while streaming scripted responses.
final class _DescribeFailingProvider implements ModelProvider {
  _DescribeFailingProvider(this.responses);

  final List<ModelResponse> responses;
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      throw StateError('describe failed');

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield const TextDeltaEvent('delta');
    yield ModelCompletedEvent(responses[_index++]);
  }
}

/// Succeeds once, then fails every subsequent model call.
final class _FirstOkThenFailProvider implements ModelProvider {
  _FirstOkThenFailProvider(this.first);

  final ModelResponse first;
  var _calls = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: 100);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (_calls++ == 0) {
      yield ModelCompletedEvent(first);
      return;
    }
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

/// A session context builder that injects [skills] for every directory.
SessionContext Function(String) _contextBuilder(SkillCatalog skills) =>
    (cwd) => SessionContext(
      workingDirectory: cwd,
      instructions: const [],
      skills: skills,
    );

final class _MemorySkillCatalog implements SkillCatalog {
  _MemorySkillCatalog(this.skills);

  final List<Skill> skills;

  @override
  List<SkillSummary> get summaries => [
    for (final skill in skills)
      if (!skill.disableModelInvocation)
        SkillSummary(
          name: skill.name,
          path: skill.path,
          description: skill.description,
        ),
  ];

  @override
  Skill? lookup(String name) {
    for (final skill in skills) {
      if (skill.name == name) {
        return skill.disableModelInvocation ? null : skill;
      }
    }
    return null;
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
    final checkpoint = value.compaction;
    final visible = checkpoint == null || checkpoint.summary.trim().isEmpty
        ? timeline
        : timeline
              .where(
                (item) => item.sequence > checkpoint.compactedThroughSequence,
              )
              .toList();
    return SessionSnapshot(
      session: value,
      turns: List.unmodifiable(turns),
      timeline: List.unmodifiable(visible),
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
  ) async {
    compaction = checkpoint;
    final value = session;
    if (value != null) {
      session = Session(
        id: value.id,
        title: value.title,
        workingDirectory: value.workingDirectory,
        additionalDirectories: value.additionalDirectories,
        createdAt: value.createdAt,
        updatedAt: value.updatedAt,
        compaction: checkpoint,
        lastUsage: value.lastUsage,
      );
    }
  }

  @override
  Future<void> deleteSession(SessionId sessionId) async => session = null;
}

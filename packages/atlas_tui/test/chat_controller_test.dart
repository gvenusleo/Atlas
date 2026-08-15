import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

void main() {
  late AgentRuntime runtime;
  late _ScriptedProvider provider;
  late DriftSessionStore defaultStore;
  var defaultStoreOpen = false;

  setUp(() {
    provider = _ScriptedProvider();
    defaultStore = DriftSessionStore.inMemory();
    defaultStoreOpen = true;
    runtime = AgentRuntime(
      store: defaultStore,
      provider: provider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      sessionContextBuilder: _contextBuilder(_Skills(['check'])),
      maxSteps: 5,
    );
  });

  tearDown(() async {
    if (defaultStoreOpen) {
      await defaultStore.close();
    }
  });

  Future<void> closeDefaultStore() async {
    await defaultStore.close();
    defaultStoreOpen = false;
  }

  test('records the user message and streams the assistant reply', () async {
    provider.toolFirst = false;
    final controller = ChatController(runtime: runtime);

    await controller.send('hello');

    expect(controller.busy, isFalse);
    expect(controller.messages.map((m) => m.kind), [
      ChatMessageKind.user,
      ChatMessageKind.assistant,
    ]);
    expect(controller.messages.first.text, 'hello');
    expect(controller.messages.last.text, 'done');
  });

  test('renders tool calls with their results', () async {
    final controller = ChatController(runtime: runtime);

    await controller.send('use the tool');

    final kinds = controller.messages.map((m) => m.kind).toList();
    expect(kinds, [
      ChatMessageKind.user,
      ChatMessageKind.tool,
      ChatMessageKind.assistant,
    ]);
    final tool = controller.messages[1];
    expect(tool.toolName, 'echo');
    expect(tool.arguments, {'fail': false});
    expect(tool.isError, isFalse);
    expect(tool.text, 'ok');
  });

  test('marks tool failures and truncates long results', () async {
    provider.failTool = true;
    final controller = ChatController(runtime: runtime);

    await controller.send('break the tool');

    final tool = controller.messages[1];
    expect(tool.isError, isTrue);
    expect(tool.text, startsWith('failed: '));
    expect(tool.text.length, lessThanOrEqualTo(maxToolResultChars + 20));
  });

  test('keeps the tail of long tool results', () async {
    provider.toolArguments = {'length': 500};
    final controller = ChatController(runtime: runtime);

    await controller.send('long result');

    final tool = controller.messages[1];
    expect(tool.text, startsWith('...'));
    expect(tool.text.length, lessThanOrEqualTo(maxToolResultChars + 3));
  });

  test('renders reasoning deltas separately', () async {
    provider.printReasoning = true;
    final controller = ChatController(runtime: runtime);

    await controller.send('think out loud');

    expect(
      controller.messages.map((m) => m.kind),
      contains(ChatMessageKind.reasoning),
    );
  });

  test('bounds the reasoning message to the tail window', () async {
    provider.printReasoning = true;
    provider.reasoningChunk = 'x' * 800;
    final controller = ChatController(runtime: runtime);

    await controller.send('think long');

    final reasoning = controller.messages.firstWhere(
      (m) => m.kind == ChatMessageKind.reasoning,
    );
    expect(reasoning.text.length, lessThanOrEqualTo(maxReasoningChars));
    expect(reasoning.text, endsWith('x' * 800));
  });

  test('reuses the session id across turns', () async {
    final controller = ChatController(runtime: runtime);

    await controller.send('first');
    final firstRun = provider.sessionIds.length;
    await controller.send('second');

    expect(provider.sessionIds, hasLength(greaterThan(firstRun)));
    expect(provider.sessionIds.toSet(), hasLength(1));
  });

  test('ignores blank text and concurrent sends', () async {
    final controller = ChatController(runtime: runtime);

    await controller.send('   ');
    expect(controller.messages, isEmpty);

    // A send while busy is dropped; the gate proves only one turn ran.
    provider.gate = Completer<void>();
    final running = controller.send('first');
    await Future<void>.delayed(Duration.zero);
    await controller.send('second');
    provider.gate!.complete();
    await running;
    expect(
      controller.messages.where((m) => m.kind == ChatMessageKind.user),
      hasLength(1),
    );
  });

  test('renders turn failures as error messages', () async {
    provider.failTurn = true;
    final controller = ChatController(runtime: runtime);

    await controller.send('boom');

    expect(controller.messages.last.kind, ChatMessageKind.error);
  });

  test('tracks the context tokens of the last finished turn', () async {
    final controller = ChatController(runtime: runtime);
    expect(controller.contextTokens, 0);

    await controller.send('hello');

    expect(controller.contextTokens, 4321);
  });

  test('flips between working and thinking while a turn runs', () async {
    provider.printReasoning = true;
    provider.gate = Completer<void>();
    final controller = ChatController(runtime: runtime);
    final running = controller.send('think');
    await Future<void>.delayed(Duration.zero);

    // The reasoning delta arrives before the gate, so the status shows the
    // thinking phase while the turn is still open.
    expect(controller.turnPhase, TurnPhase.thinking);
    expect(controller.busy, isTrue);

    controller.cancelTurn();
    provider.gate!.complete();
    await running;
    expect(controller.turnPhase, TurnPhase.idle);
    expect(controller.busy, isFalse);
  });

  test('shows a compaction notice after a long session', () async {
    await closeDefaultStore();
    final compactingProvider = _ScriptedProvider()
      ..contextWindow = 10000
      ..inputTokens = 9500;
    final compactingRuntime = AgentRuntime(
      store: _testStore(),
      provider: compactingProvider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      maxSteps: 5,
      keptRecentTurns: 1,
    );
    final controller = ChatController(runtime: compactingRuntime);

    await controller.send('first');
    await controller.send('second');

    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(
      controller.messages.last.text,
      'Context compacted. Kept 2 recent messages.',
    );
  });

  test('compact forwards an instruction to the summary request', () async {
    await closeDefaultStore();
    final compactingProvider = _ScriptedProvider()..contextWindow = 10000;
    final compactingRuntime = AgentRuntime(
      store: _testStore(),
      provider: compactingProvider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      maxSteps: 5,
      keptRecentTurns: 1,
    );
    final controller = ChatController(runtime: compactingRuntime);
    await controller.send('first');
    await controller.send('second');

    await controller.compact(instruction: 'keep files');

    final summaryPrompt = textFromContent(
      compactingProvider.lastMessages!.single.content,
    );
    expect(summaryPrompt, contains('Additional user instruction:\nkeep files'));
    expect(controller.messages.last.text, startsWith('Context compacted.'));
  });

  test('compact without a session shows a notice', () async {
    final controller = ChatController(runtime: runtime);

    await controller.compact();

    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(controller.messages.last.text, 'No session to compact.');
  });

  test('shows compacting in the status line during compaction', () async {
    await closeDefaultStore();
    final compactingProvider = _ScriptedProvider()
      ..contextWindow = 10000
      ..inputTokens = 9500;
    final compactingRuntime = AgentRuntime(
      store: _testStore(),
      provider: compactingProvider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      maxSteps: 5,
      keptRecentTurns: 1,
    );
    final controller = ChatController(runtime: compactingRuntime);
    await controller.send('first');

    compactingProvider.compactionGate = Completer<void>();
    final second = controller.send('second');
    for (
      var i = 0;
      i < 200 && controller.turnPhase != TurnPhase.compacting;
      i++
    ) {
      await Future<void>.delayed(const Duration(milliseconds: 5));
    }
    expect(controller.turnPhase, TurnPhase.compacting);
    expect(controller.busy, isTrue);

    compactingProvider.compactionGate!.complete();
    await second;
    expect(controller.turnPhase, TurnPhase.idle);
  });

  test('compact command compacts the session', () async {
    await closeDefaultStore();
    final compactingProvider = _ScriptedProvider()..contextWindow = 10000;
    final compactingRuntime = AgentRuntime(
      store: _testStore(),
      provider: compactingProvider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      maxSteps: 5,
      keptRecentTurns: 1,
    );
    final controller = ChatController(runtime: compactingRuntime);

    await controller.send('first');
    await controller.send('second');
    await controller.compact();

    expect(controller.turnPhase, TurnPhase.idle);
    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(
      controller.messages.last.text,
      'Context compacted. Kept 2 recent messages.',
    );
  });

  test('cancelTurn interrupts manual compaction', () async {
    await closeDefaultStore();
    final compactingProvider = _ScriptedProvider()..contextWindow = 10000;
    final compactingRuntime = AgentRuntime(
      store: _testStore(),
      provider: compactingProvider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      maxSteps: 5,
      keptRecentTurns: 1,
    );
    final controller = ChatController(runtime: compactingRuntime);
    await controller.send('first');
    await controller.send('second');
    compactingProvider.compactionGate = Completer<void>();

    final compacting = controller.compact();
    expect(controller.turnPhase, TurnPhase.compacting);
    controller.cancelTurn();
    await compacting;

    expect(controller.turnPhase, TurnPhase.idle);
    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(controller.messages.last.text, 'Compaction cancelled');
  });

  test('compact shows a notice when nothing can be compacted', () async {
    final controller = ChatController(runtime: runtime);

    await controller.send('hello');
    await controller.compact();

    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(controller.messages.last.text, 'Nothing to compact');
  });

  test('cancelTurn interrupts a running turn', () async {
    provider.gate = Completer<void>();
    final controller = ChatController(runtime: runtime);
    final running = controller.send('slow');
    await Future<void>.delayed(Duration.zero);
    expect(controller.turnPhase, TurnPhase.working);

    controller.cancelTurn();
    provider.gate!.complete();
    await running;

    expect(controller.turnPhase, TurnPhase.idle);
    expect(controller.messages.last.kind, isNot(ChatMessageKind.error));
    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(controller.messages.last.text, 'Turn cancelled');
  });

  test(
    'setModel stores the override and effort for subsequent turns',
    () async {
      final controller = ChatController(runtime: runtime);
      final ref = ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('reasoner'),
      );

      controller.setModel(
        ref,
        displayName: 'Reasoner',
        effort: 'high',
        effortName: 'high',
      );
      expect(controller.model, ref);
      expect(controller.reasoningEffort, 'high');
      expect(
        controller.messages.last.text,
        'Switched to Reasoner (fake), effort high',
      );

      await controller.send('hello');
      expect(provider.lastModel, ref);
      expect(provider.lastReasoningEffort, 'high');
    },
  );

  test('passes selected skills to the turn', () async {
    final controller = ChatController(runtime: runtime);

    await controller.send('/check review', selectedSkills: ['check']);

    expect(provider.lastMessages!.first.role, ModelMessageRole.user);
    expect(
      textFromContent(provider.lastMessages!.first.content),
      contains('<skill>\n<name>check</name>'),
    );
  });

  test('reset clears the transcript and the context tokens', () async {
    final controller = ChatController(runtime: runtime);
    await controller.send('hello');
    expect(controller.contextTokens, 4321);

    controller.reset();

    expect(controller.messages, isEmpty);
    expect(controller.contextTokens, 0);
    expect(controller.model, isNull);
  });

  test('reset keeps the model override but clears the transcript', () async {
    final controller = ChatController(runtime: runtime);
    controller.setModel(
      ModelRef(providerId: ProviderId('fake'), modelId: ModelId('x')),
    );

    controller.reset();

    expect(controller.messages, isEmpty);
    expect(controller.model, isNotNull);
  });

  test('listSessions filters by the working directory', () async {
    final other = ChatController(runtime: runtime, workingDirectory: '/other');
    await other.send('from elsewhere');

    final local = ChatController(runtime: runtime);
    final localPage = await local.listSessions();
    expect(localPage.items, isEmpty);

    // The runtime still lists every session without a directory filter.
    final allPage = await runtime.listSessions();
    expect(allPage.items, hasLength(1));
    expect(allPage.items.single.title, 'from elsewhere');
  });

  test('resume restores the transcript and session state', () async {
    final controller = ChatController(runtime: runtime);
    await controller.send('hello');
    final summary = (await controller.listSessions()).items.single;
    expect(summary.title, 'hello');

    controller.reset();
    expect(controller.messages, isEmpty);

    expect(await controller.resume(summary.id), isTrue);

    expect(controller.messages.map((m) => m.kind), [
      ChatMessageKind.user,
      ChatMessageKind.tool,
      ChatMessageKind.assistant,
    ]);
    expect(controller.messages.first.text, 'hello');
    expect(controller.messages.last.text, 'done');
    expect(controller.contextTokens, 4321);
  });

  test('resume keeps new turns in the resumed session', () async {
    final controller = ChatController(runtime: runtime);
    await controller.send('first');
    final firstId = (await controller.listSessions()).items.single.id;
    controller.reset();
    await controller.resume(firstId);

    final before = provider.sessionIds.length;
    await controller.send('second');

    expect(provider.sessionIds, hasLength(before + 1));
    expect(provider.sessionIds.last, firstId.value);
  });

  test('resume switches the working directory to the session', () async {
    final other = ChatController(runtime: runtime, workingDirectory: '/other');
    await other.send('from elsewhere');
    final id = (await other.listSessions()).items.single.id;

    final controller = ChatController(runtime: runtime);
    expect(controller.workingDirectory, isNot('/other'));

    await controller.resume(id);

    expect(controller.workingDirectory, '/other');
  });

  test('resume reports load failures as error messages', () async {
    final controller = ChatController(runtime: runtime);

    expect(await controller.resume(SessionId('missing')), isFalse);

    expect(controller.messages.last.kind, ChatMessageKind.error);
    expect(controller.messages.last.text, contains('Session not found'));
  });

  test('resume announces a previous compaction', () async {
    await closeDefaultStore();
    final compactingProvider = _ScriptedProvider()
      ..contextWindow = 10000
      ..inputTokens = 9500;
    final compactingRuntime = AgentRuntime(
      store: _testStore(),
      provider: compactingProvider,
      tools: LocalToolRegistry([_EchoTool()]),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      maxSteps: 5,
      keptRecentTurns: 1,
    );
    final writer = ChatController(runtime: compactingRuntime);
    await writer.send('first');
    await writer.send('second');
    final id = (await writer.listSessions()).items.single.id;

    final controller = ChatController(runtime: compactingRuntime);
    await controller.resume(id);

    expect(controller.messages.last.kind, ChatMessageKind.system);
    expect(
      controller.messages.last.text,
      'Context compacted. Kept 2 recent messages.',
    );
  });

  test('messagesFromTimeline renders and pairs tool items', () {
    final sessionId = SessionId('s');
    final turnId = TurnId('t');
    final at = DateTime.utc(2026, 1, 1);
    TimelineItemId nextId(int sequence) => TimelineItemId('i$sequence');
    final timeline = <TimelineItem>[
      UserMessageItem(
        id: nextId(1),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 1,
        occurredAt: at,
        content: [TextContent('hi')],
      ),
      AssistantMessageItem(
        id: nextId(2),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 2,
        occurredAt: at,
        content: [TextContent('let me check')],
        model: ModelRef(providerId: ProviderId('p'), modelId: ModelId('m')),
        stopReason: StopReason.toolUse,
      ),
      ToolCallItem(
        id: nextId(3),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 3,
        occurredAt: at,
        call: ToolCall(
          id: ToolCallId('c1'),
          name: 'read',
          arguments: {'path': '/a'},
        ),
      ),
      ToolResultItem(
        id: nextId(4),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 4,
        occurredAt: at,
        callId: ToolCallId('c1'),
        content: 'content of a',
      ),
    ];

    final messages = messagesFromTimeline(timeline);

    expect(messages.map((m) => m.kind), [
      ChatMessageKind.user,
      ChatMessageKind.assistant,
      ChatMessageKind.tool,
    ]);
    expect(messages.first.text, 'hi');
    expect(messages[1].text, 'let me check');
    expect(messages[2].toolName, 'read');
    expect(messages[2].arguments, {'path': '/a'});
    expect(messages[2].text, 'content of a');
    expect(messages[2].isError, isFalse);
  });

  test('messagesFromTimeline marks failed tool results', () {
    final sessionId = SessionId('s');
    final turnId = TurnId('t');
    final at = DateTime.utc(2026, 1, 1);
    final timeline = <TimelineItem>[
      ToolCallItem(
        id: TimelineItemId('a'),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 1,
        occurredAt: at,
        call: ToolCall(
          id: ToolCallId('c1'),
          name: 'shell',
          arguments: const {},
        ),
      ),
      ToolResultItem(
        id: TimelineItemId('b'),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 2,
        occurredAt: at,
        callId: ToolCallId('c1'),
        content: 'boom',
        isError: true,
      ),
    ];

    final messages = messagesFromTimeline(timeline);

    expect(messages.single.kind, ChatMessageKind.tool);
    expect(messages.single.text, 'failed: boom');
    expect(messages.single.isError, isTrue);
  });
}

/// Creates an in-memory store and closes it after the current test.
DriftSessionStore _testStore() {
  final store = DriftSessionStore.inMemory();
  addTearDown(store.close);
  return store;
}

/// Echoes a tool result so tool success paths run without real tools.
final class _EchoTool implements Tool {
  @override
  ToolDescriptor get descriptor =>
      const ToolDescriptor(name: 'echo', description: '', inputSchema: {});

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    final length = arguments['length'];
    final content = length is int && length > 0
        ? 'x' * length
        : arguments['fail'] == true
        ? 'nope'
        : 'ok';
    return ToolResult(content: content, isError: arguments['fail'] == true);
  }
}

/// A session context builder that injects [skills] for every directory.
SessionContext Function(String) _contextBuilder(SkillCatalog skills) =>
    (cwd) => SessionContext(
      workingDirectory: cwd,
      instructions: const [],
      skills: skills,
    );

/// In-memory catalog exposing skills by name from a fixed set.
final class _Skills implements SkillCatalog {
  _Skills(this.names);

  final List<String> names;

  @override
  List<SkillSummary> get summaries => [
    for (final name in names)
      SkillSummary(
        name: name,
        path: '/skills/$name/SKILL.md',
        description: '$name skill.',
      ),
  ];

  @override
  Skill? lookup(String name) => names.contains(name)
      ? Skill(
          name: name,
          description: '$name skill.',
          dir: '/skills/$name',
          path: '/skills/$name/SKILL.md',
          content: '# $name\n\nFollow these steps.',
        )
      : null;
}

/// Scripted provider: first request asks for a tool call, later requests
/// finish the turn. Records every session id it receives.
final class _ScriptedProvider implements ModelProvider {
  int _requests = 0;
  bool failTool = false;
  bool failTurn = false;
  bool printReasoning = false;
  String reasoningChunk = 'thinking hard';
  bool toolFirst = true;
  Completer<void>? gate;

  /// Extra arguments merged into the first tool call.
  Map<String, Object?> toolArguments = const {};
  final sessionIds = <String>[];
  ModelRef? lastModel;
  String? lastReasoningEffort;
  List<ModelMessage>? lastMessages;

  /// Model context window reported by [describe]; zero disables compaction.
  int contextWindow = 0;

  /// Input tokens reported by the final response of each turn.
  int inputTokens = 0;

  /// Blocks the compaction request when set.
  Completer<void>? compactionGate;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: contextWindow);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    sessionIds.add(request.sessionId.value);
    lastModel = request.model;
    lastReasoningEffort = request.reasoningEffort;
    lastMessages = request.messages;
    _requests++;
    final compaction =
        request.messages.length == 1 &&
        textFromContent(
          request.messages.single.content,
        ).contains('<transcript>');
    if (compaction) {
      final gate = compactionGate;
      if (gate != null) {
        await Future.any<void>([
          gate.future,
          if (request.cancellation != null) request.cancellation!.whenCancelled,
        ]);
        request.cancellation?.throwIfCancelled();
      }
    }
    if (failTurn) {
      throw StateError('provider exploded');
    }
    if (printReasoning) {
      yield ReasoningDeltaEvent(reasoningChunk);
    }
    if (gate != null) {
      final cancellation = request.cancellation;
      await Future.any<void>([
        gate!.future,
        cancellation?.whenCancelled ?? Future<void>.value(),
      ]);
      if (cancellation?.isCancelled == true) {
        throw const TurnCancelledException();
      }
      return;
    }
    if (_requests == 1 && toolFirst) {
      yield ModelCompletedEvent(
        ModelResponse(
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'echo',
              arguments: {'fail': failTool, ...toolArguments},
            ),
          ],
          stopReason: StopReason.toolUse,
        ),
      );
      return;
    }
    // Stream the final answer as deltas so the controller accumulates text.
    yield const TextDeltaEvent('do');
    yield const TextDeltaEvent('ne');
    yield ModelCompletedEvent(
      ModelResponse(
        content: const [TextContent('done')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(inputTokens: inputTokens, totalTokens: 4321),
      ),
    );
  }
}

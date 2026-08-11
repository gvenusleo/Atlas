import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

void main() {
  late AgentRuntime runtime;
  late _ScriptedProvider provider;

  setUp(() {
    provider = _ScriptedProvider();
    runtime = AgentRuntime(
      store: DriftSessionStore.inMemory(),
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

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    sessionIds.add(request.sessionId.value);
    lastModel = request.model;
    lastReasoningEffort = request.reasoningEffort;
    lastMessages = request.messages;
    _requests++;
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
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('done')],
        stopReason: StopReason.endTurn,
        usage: TokenUsage(totalTokens: 4321),
      ),
    );
  }
}

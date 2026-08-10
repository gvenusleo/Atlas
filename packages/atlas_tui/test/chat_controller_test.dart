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

  test('renders reasoning deltas separately', () async {
    provider.printReasoning = true;
    final controller = ChatController(runtime: runtime);

    await controller.send('think out loud');

    expect(
      controller.messages.map((m) => m.kind),
      contains(ChatMessageKind.reasoning),
    );
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
}

/// Echoes a tool result so tool success paths run without real tools.
final class _EchoTool implements Tool {
  @override
  ToolDescriptor get descriptor =>
      const ToolDescriptor(name: 'echo', description: '', inputSchema: {});

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async =>
      ToolResult(
        content: arguments['fail'] == true ? 'nope' : 'ok',
        isError: arguments['fail'] == true,
      );
}

/// Scripted provider: first request asks for a tool call, later requests
/// finish the turn. Records every session id it receives.
final class _ScriptedProvider implements ModelProvider {
  int _requests = 0;
  bool failTool = false;
  bool failTurn = false;
  bool printReasoning = false;
  bool toolFirst = true;
  Completer<void>? gate;
  final sessionIds = <String>[];

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    sessionIds.add(request.sessionId.value);
    if (gate != null) {
      await gate!.future;
      return;
    }
    _requests++;
    if (failTurn) {
      throw StateError('provider exploded');
    }
    if (printReasoning) {
      yield const ReasoningDeltaEvent('thinking hard');
    }
    if (_requests == 1 && toolFirst) {
      yield ModelCompletedEvent(
        ModelResponse(
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'echo',
              arguments: {'fail': failTool},
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
      ),
    );
  }
}

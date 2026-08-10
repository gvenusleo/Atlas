import 'dart:async';
import 'dart:io';

import 'package:atlas_cli/atlas_cli.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

void main() {
  test('composeRuntime wires config into a working runtime', () {
    final config = parseConfig('''
default_model: oa/gpt-4o
providers:
  - name: oa
    type: responses
    base_url: https://example.com
    api_key: k
    models:
      - value: gpt-4o
''');
    final runtime = composeRuntime(
      config,
      store: DriftSessionStore.inMemory(),
      tools: LocalToolRegistry(const []),
    );

    expect(runtime.defaultModel, config.defaultModel);
    expect(runtime.maxSteps, config.agent.maxSteps);
    expect(runtime.temperature, config.agent.temperature);
  });

  test('builds real providers and storage from config', () async {
    final config = parseConfig('''
default_model: oa/gpt-4o
providers:
  - name: oa
    type: responses
    base_url: https://example.com
    api_key: k
    models:
      - value: gpt-4o
''');
    final runtime = composeRuntime(config, store: DriftSessionStore.inMemory());

    // The config-driven provider branch is exercised: the composite provider
    // resolves the configured default model without any HTTP traffic.
    final descriptor = await runtime.provider.describe(runtime.defaultModel);
    expect(descriptor.ref, runtime.defaultModel);
    expect(descriptor.inputCapabilities, {ModelInputCapability.text});
  });

  test(
    'runs a multi-turn tool loop with a fake provider and real tools',
    () async {
      final config = parseConfig('''
default_model: fake/provider
providers:
  - name: fake
    type: responses
    base_url: https://example.com
    api_key: k
    models:
      - value: provider
''');
      final dir = await Directory.systemTemp.createTemp('compose_test_');
      addTearDown(() => dir.delete(recursive: true));
      await Directory('${dir.path}/sub').create(recursive: true);
      await File('${dir.path}/sub/note.txt').writeAsString('hello world');

      final fakeProvider = _FakeProvider();
      final runtime = composeRuntime(
        config,
        store: DriftSessionStore.inMemory(),
        tools: LocalToolRegistry([ReadTool(), ShellTool()]),
        provider: fakeProvider,
      );
      final events = await runtime
          .run(
            TurnRequest(
              content: const [TextContent('read the note')],
              workingDirectory: dir.path,
            ),
          )
          .toList();

      // The composed runtime injected the real system prompt builder.
      expect(fakeProvider.prompts, isNotEmpty);
      expect(
        fakeProvider.prompts.first,
        contains('Working directory: ${dir.path}'),
      );
      expect(
        fakeProvider.prompts.first,
        contains('Available tools: read, shell.'),
      );

      expect(
        events.map((e) => e.runtimeType),
        containsAll([
          TurnStarted,
          ModelResponseReceived,
          ToolStarted,
          ToolFinished,
          TurnFinished,
        ]),
      );
      // The loop ran two model steps: the tool request and the final answer.
      expect(events.whereType<ModelResponseReceived>().length, 2);
      final finish = events.last as TurnFinished;
      expect(finish.outcome.status, TurnStatus.completed);
    },
  );
}

// Fake provider drives the tool loop: the first request asks to read the file,
// every later request (which carries the tool result) finishes the turn. It
// records every system prompt it receives so tests can assert the wiring.
final class _FakeProvider implements ModelProvider {
  _FakeProvider();

  /// The system prompts received per model request.
  final prompts = <String>[];

  @override
  Future<ModelDescriptor> describe(ModelRef model) async => ModelDescriptor(
    ref: model,
    inputCapabilities: const {ModelInputCapability.text},
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    prompts.add(request.systemPrompt);
    final hasToolCall = request.messages.any(
      (message) => message.toolCalls.isNotEmpty,
    );
    if (!hasToolCall) {
      yield ModelCompletedEvent(
        ModelResponse(
          content: const [],
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'read',
              arguments: {'path': 'sub/note.txt'},
            ),
          ],
          stopReason: StopReason.toolUse,
        ),
      );
      return;
    }
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('done reading')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  test('renders the chat surface and submits a turn', () async {
    await testNocterm('chat app', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = AgentRuntime(
        store: DriftSessionStore.inMemory(),
        provider: provider,
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: ModelRef(
          providerId: ProviderId('fake'),
          modelId: ModelId('model'),
        ),
      );
      await tester.pumpComponent(
        AtlasTuiApp(runtime: runtime, workingDirectory: '/tmp'),
      );

      expect(tester.terminalState, containsText('Message Atlas'));

      await tester.enterText('hello there');
      await tester.sendEnter();
      // The focused input cursor keeps scheduling frames, so pumpAndSettle
      // would never settle; pump a bounded number of frames instead while the
      // asynchronous turn completes.
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      expect(tester.terminalState, containsText('hello there'));
      expect(provider.streamCalls, greaterThan(0));
    });
  });

  test('keeps the draft input while a turn is running', () async {
    await testNocterm('chat app busy input', (tester) async {
      final provider = _ScriptedProvider()..gate = Completer<void>();
      final runtime = AgentRuntime(
        store: DriftSessionStore.inMemory(),
        provider: provider,
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: ModelRef(
          providerId: ProviderId('fake'),
          modelId: ModelId('model'),
        ),
      );
      await tester.pumpComponent(
        AtlasTuiApp(runtime: runtime, workingDirectory: '/tmp'),
      );

      await tester.enterText('pending draft');
      await tester.sendEnter();
      await tester.pump();

      // The turn is still running and the draft is not cleared.
      expect(tester.terminalState, containsText('pending draft'));
      expect(provider.streamCalls, 1);

      provider.gate!.complete();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
    });
  });
}

/// Answers every turn with a finished response after streaming one delta.
final class _ScriptedProvider implements ModelProvider {
  int streamCalls = 0;
  Completer<void>? gate;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    streamCalls++;
    if (gate != null) {
      await gate!.future;
    }
    yield const TextDeltaEvent('hi');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('hi')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

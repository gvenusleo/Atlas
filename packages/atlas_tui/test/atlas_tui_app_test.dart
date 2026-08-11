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
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
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
      // The status line below the input bar reflects the default model and
      // the context usage accumulated by the turn.
      expect(tester.terminalState, containsText('  model'));
      expect(tester.terminalState, containsText('Context 0% used'));
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
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
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

  test('shows the slash popup while typing a command', () async {
    await testNocterm('slash popup', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      expect(tester.terminalState, containsText('/model'));
      expect(tester.terminalState, containsText('/new'));
      expect(tester.terminalState, containsText('/quit'));
    });
  });

  test('arrow keys move the popup selection and enter fills it in', () async {
    await testNocterm('slash popup selection', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      // `/new ` replaces the token and the popup closes.
      expect(tester.terminalState, containsText('/new'));
      expect(tester.terminalState, isNot(containsText('/model')));
    });
  });

  test('escape dismisses the popup until the draft changes', () async {
    await testNocterm('slash popup dismiss', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      await tester.sendEscape();
      await tester.pump();
      expect(tester.terminalState, isNot(containsText('/new')));

      // The same draft stays dismissed; a change reopens the popup.
      await tester.sendEscape();
      await tester.pump();
      expect(tester.terminalState, isNot(containsText('/new')));
      await tester.enterText('n');
      await tester.pump();
      expect(tester.terminalState, containsText('/new'));
    });
  });

  test('/new clears the transcript and starts a fresh session', () async {
    await testNocterm('slash new', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('hello there');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      expect(tester.terminalState, containsText('hello there'));

      await tester.enterText('/new');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      expect(tester.terminalState, isNot(containsText('hello there')));
    });
  });

  test('/model opens the picker and enter switches the model', () async {
    await testNocterm('slash model', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/model');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();

      // The picker lists models by display name under a heading.
      expect(tester.terminalState, containsText('Select model'));
      expect(tester.terminalState, containsText('Alpha model'));
      expect(tester.terminalState, containsText('Beta model'));

      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      expect(tester.terminalState, containsText('Switched to Beta model'));
    });
  });

  test(
    '/model advances to the reasoning effort stage for multi-effort models',
    () async {
      await testNocterm('slash model effort', (tester) async {
        final provider = _ScriptedProvider();
        final runtime = _runtime(provider);
        await tester.pumpComponent(
          AtlasTuiApp(
            runtime: runtime,
            models: _reasoningModels,
            workingDirectory: '/tmp',
          ),
        );

        await tester.enterText('/model');
        await tester.sendEnter();
        await tester.pump();
        await tester.sendEnter();
        await tester.pump();

        // Select the first model (deep) and enter the effort stage.
        await tester.sendEnter();
        await tester.pump();
        expect(
          tester.terminalState,
          containsText('Select reasoning effort for Deep model'),
        );
        expect(tester.terminalState, containsText('Low effort'));
        expect(tester.terminalState, containsText('High effort'));

        await tester.sendArrowDown();
        await tester.pump();
        await tester.sendEnter();
        await tester.pump();

        expect(
          tester.terminalState,
          containsText('Switched to Deep model (fake), effort High effort'),
        );
      });
    },
  );

  test('/model applies a single-effort model directly', () async {
    await testNocterm('slash model single effort', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _reasoningModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/model');
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      // Move to the second model (single) and confirm: no effort stage.
      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      expect(
        tester.terminalState,
        containsText('Switched to Single model (fake), effort Medium'),
      );
      expect(
        tester.terminalState,
        isNot(containsText('Select reasoning effort')),
      );
    });
  });

  test('/model escape cancels without switching', () async {
    await testNocterm('slash model cancel', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/model');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEscape();
      await tester.pump();

      expect(tester.terminalState, isNot(containsText('Switched to')));
    });
  });

  test('/quit invokes the quit callback', () async {
    await testNocterm('slash quit', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      var quitCount = 0;
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
          onQuit: () => quitCount++,
        ),
      );

      await tester.enterText('/quit');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      expect(quitCount, 1);
    });
  });

  test('unknown commands submit as normal messages', () async {
    await testNocterm('slash unknown', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/nope');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      expect(provider.streamCalls, 1);
    });
  });
}

AgentRuntime _runtime(_ScriptedProvider provider) => AgentRuntime(
  store: DriftSessionStore.inMemory(),
  provider: provider,
  tools: LocalToolRegistry(const []),
  ids: SecureIdGenerator(),
  defaultModel: ModelRef(
    providerId: ProviderId('fake'),
    modelId: ModelId('model'),
  ),
);

/// Shared test models for `/model` switching.
final _testModels = [
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('alpha')),
    name: 'Alpha model',
  ),
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('beta')),
    name: 'Beta model',
  ),
];

/// Models exercising the reasoning-effort stages of `/model`.
final _reasoningModels = [
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('deep')),
    name: 'Deep model',
    reasoningEfforts: const [
      ReasoningEffortOption(value: 'low', name: 'Low effort'),
      ReasoningEffortOption(value: 'high', name: 'High effort'),
    ],
  ),
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('single')),
    name: 'Single model',
    reasoningEfforts: const [
      ReasoningEffortOption(value: 'medium', name: 'Medium'),
    ],
  ),
];

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

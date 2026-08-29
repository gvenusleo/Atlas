import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_state.dart';

import 'workspace_support.dart';

void main() {
  test('streams a runtime turn into persisted workspace messages', () async {
    final model = ModelDescriptor(
      ref: ModelRef(
        providerId: ProviderId('test'),
        modelId: ModelId('streaming'),
      ),
      name: 'Streaming test model',
      reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [model],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('show me the stream');

    final state = container.read(workspaceProvider);
    expect(state.messages.map((message) => message.kind), [
      WorkspaceMessageKind.user,
      WorkspaceMessageKind.reasoning,
      WorkspaceMessageKind.assistant,
    ]);
    expect(state.messages[1].text, 'first thought');
    expect(state.messages[1].isRunning, isFalse);
    expect(state.messages[2].text, 'Hello from Atlas.');
    expect(state.sessionId, isNotNull);
    // The turn finished: status fields are reset.  [turnStartedAt]
    expect(state.active.turnPhase, TurnPhase.idle);
    expect(state.active.turnStartedAt, isNull);
    expect(state.active.busy, isFalse);
    await controller.refreshSessions();
    expect(container.read(workspaceProvider).sessions, hasLength(1));
  });
  test('turn phase follows the model stream and resets on finish', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('gated')),
    );
    final provider = GatedFakeProvider(model.ref);
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [model],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    // Keep a listener so Riverpod does not reset the notifier state while
    // the turn is in flight without any widget subscribing to it.
    container.listen(workspaceProvider, (_, _) {});
    final turn = controller.send('hello');
    await pumpEventQueue();
    expect(container.read(workspaceProvider).active.busy, isTrue);
    expect(
      container.read(workspaceProvider).active.turnPhase,
      TurnPhase.thinking,
    );
    expect(container.read(workspaceProvider).active.turnStartedAt, isNotNull);

    provider.reasoningGate.complete();
    await pumpEventQueue();
    expect(
      container.read(workspaceProvider).active.turnPhase,
      TurnPhase.working,
    );

    provider.textGate.complete();
    await turn;
    expect(container.read(workspaceProvider).active.busy, isFalse);
    expect(container.read(workspaceProvider).active.turnPhase, TurnPhase.idle);
    expect(container.read(workspaceProvider).active.turnStartedAt, isNull);
  });
  test('resume restores persisted reasoning messages', () async {
    final model = ModelDescriptor(
      ref: ModelRef(
        providerId: ProviderId('test'),
        modelId: ModelId('streaming'),
      ),
      name: 'Streaming test model',
      reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [model],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('show me the stream');
    final sessionId = container.read(workspaceProvider).sessionId!;

    controller.newSession();
    await controller.resume(sessionId);

    final state = container.read(workspaceProvider);
    expect(state.messages.map((message) => message.kind), [
      WorkspaceMessageKind.user,
      WorkspaceMessageKind.reasoning,
      WorkspaceMessageKind.assistant,
    ]);
    expect(state.messages[1].text, 'first thought');
    expect(state.messages[2].text, 'Hello from Atlas.');
  });
  test(
    'selectModel warns when history has images and the model cannot accept them',
    () async {
      final visionModel = ModelDescriptor(
        ref: ModelRef(
          providerId: ProviderId('test'),
          modelId: ModelId('vision'),
        ),
        name: 'Vision model',
        inputCapabilities: const {
          ModelInputCapability.text,
          ModelInputCapability.image,
        },
      );
      final textModel = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('text')),
        name: 'Text only',
      );
      final store = DriftSessionStore.inMemory();
      final runtime = AgentRuntime(
        store: store,
        provider: FakeProvider(visionModel.ref),
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: visionModel.ref,
      );
      final container = ProviderContainer(
        overrides: [
          runtimeEnvironmentProvider.overrideWith(
            () => RuntimeEnvironmentController(
              local: RuntimeEnvironment(
                runtime: runtime,
                models: [visionModel, textModel],
                skills: EmptySkillCatalog(),
              ),
            ),
          ),
          workspaceWorkingDirectoryProvider.overrideWith(
            () => FixedWorkingDirectory('/tmp'),
          ),
        ],
      );
      addTearDown(store.close);
      addTearDown(container.dispose);

      final session = await runtime.createSession(workingDirectory: '/tmp');
      await runtime
          .run(
            TurnRequest(
              sessionId: session.id,
              content: const [
                TextContent('describe this'),
                ImageContent(source: 'data:image/png;base64,AAAA'),
              ],
              workingDirectory: '/tmp',
              model: visionModel.ref,
            ),
          )
          .toList();

      final controller = container.read(workspaceProvider.notifier);
      await controller.resume(session.id);
      expect(container.read(workspaceProvider).hasImages, isTrue);

      controller.selectModel(textModel);

      final state = container.read(workspaceProvider);
      expect(state.activeModel.ref, textModel.ref);
      expect(state.messages.last.kind, WorkspaceMessageKind.notice);
      expect(state.messages.last.text, contains('does not support images'));
    },
  );
  test('selectModel stays quiet for text-only history', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final textModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('text')),
      name: 'Text only',
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: FakeProvider(visionModel.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: visionModel.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [visionModel, textModel],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    final session = await runtime.createSession(workingDirectory: '/tmp');
    await runtime
        .run(
          TurnRequest(
            sessionId: session.id,
            content: const [TextContent('plain text')],
            workingDirectory: '/tmp',
            model: visionModel.ref,
          ),
        )
        .toList();

    final controller = container.read(workspaceProvider.notifier);
    await controller.resume(session.id);
    expect(container.read(workspaceProvider).hasImages, isFalse);

    controller.selectModel(textModel);

    final state = container.read(workspaceProvider);
    expect(state.activeModel.ref, textModel.ref);
    expect(state.messages.last.kind, isNot(WorkspaceMessageKind.notice));
  });
  test(
    'resume falls back to the default model when the last model is gone',
    () async {
      final modelA = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('a')),
        name: 'Model A',
        reasoningEfforts: const [ReasoningEffortOption(value: 'low')],
      );
      final modelB = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('b')),
        name: 'Model B',
        reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
      );
      final store = DriftSessionStore.inMemory();
      addTearDown(store.close);

      final firstRuntime = AgentRuntime(
        store: store,
        provider: RecordingProvider(),
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: modelA.ref,
      );
      final first = ProviderContainer(
        overrides: [
          runtimeEnvironmentProvider.overrideWith(
            () => RuntimeEnvironmentController(
              local: RuntimeEnvironment(
                runtime: firstRuntime,
                models: [modelA, modelB],
                skills: EmptySkillCatalog(),
              ),
            ),
          ),
          workspaceWorkingDirectoryProvider.overrideWith(
            () => FixedWorkingDirectory('/tmp'),
          ),
        ],
      );
      addTearDown(first.dispose);
      final firstController = first.read(workspaceProvider.notifier);
      firstController.selectModel(modelB);
      await firstController.send('keep this session');
      final sessionId = first.read(workspaceProvider).sessionId!;

      final secondRuntime = AgentRuntime(
        store: store,
        provider: RecordingProvider(),
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: modelA.ref,
      );
      final second = ProviderContainer(
        overrides: [
          runtimeEnvironmentProvider.overrideWith(
            () => RuntimeEnvironmentController(
              local: RuntimeEnvironment(
                runtime: secondRuntime,
                models: [modelA],
                skills: EmptySkillCatalog(),
              ),
            ),
          ),
          workspaceWorkingDirectoryProvider.overrideWith(
            () => FixedWorkingDirectory('/tmp'),
          ),
        ],
      );
      addTearDown(second.dispose);
      await second.read(workspaceProvider.notifier).resume(sessionId);
      expect(second.read(workspaceProvider).activeModel.ref, modelA.ref);
      expect(second.read(workspaceProvider).reasoningEffort, 'low');
    },
  );
}

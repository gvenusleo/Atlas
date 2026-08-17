import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';

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
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
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
    expect(state.messages[2].text, 'Hello from Atlas.');
    expect(state.sessionId, isNotNull);
    await controller.refreshSessions();
    expect(container.read(workspaceProvider).sessions, hasLength(1));
  });

  test('switching the working directory resets the workspace', () async {
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
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    container.read(workspaceWorkingDirectoryProvider.notifier).set('/tmp2');

    final state = container.read(workspaceProvider);
    expect(state.workingDirectory, '/tmp2');
    expect(state.messages, isEmpty);
    expect(state.sessionId, isNull);
  });

  test('refreshSessions loads sessions across all directories', () async {
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
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    final controller = container.read(workspaceProvider.notifier);
    await controller.send('first session');
    container.read(workspaceWorkingDirectoryProvider.notifier).set('/tmp2');
    await controller.send('second session');

    await controller.refreshSessions();
    final sessions = container.read(workspaceProvider).sessions;
    expect(sessions, hasLength(2));
    expect(sessions.map((session) => session.workingDirectory).toSet(), {
      '/tmp',
      '/tmp2',
    });
  });
}

/// Working directory fixed for tests.
final class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

final class _FakeProvider implements ModelProvider {
  _FakeProvider(this.model);

  final ModelRef model;

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    expect(request.model, model);
    yield const ReasoningDeltaEvent('first thought');
    yield const TextDeltaEvent('Hello ');
    yield const TextDeltaEvent('from Atlas.');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class _EmptySkillCatalog implements SkillCatalog {
  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}

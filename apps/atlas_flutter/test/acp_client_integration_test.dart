import 'dart:async';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:stream_channel/stream_channel.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_state.dart';

void main() {
  test('draft workspace uses the seeded ACP default model', () async {
    final model = ModelDescriptor(
      ref: ModelRef(
        providerId: ProviderId('anthropic'),
        modelId: ModelId('claude-sonnet'),
      ),
      name: 'Claude Sonnet',
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final wire = StreamChannelController<String>();
    final server = AcpServer(runtime, models: [model]);
    final serverDone = server.serveChannel(wire.local);
    final client = AcpClient.channel(
      wire.foreign,
      catalog: [model],
      defaultModel: model.ref,
    );
    await client.connect();
    addTearDown(() => serverDone);
    addTearDown(store.close);
    addTearDown(client.close);

    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: client,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(container.dispose);
    container.read(workspaceProvider.notifier);
    container.listen(workspaceProvider, (_, _) {});

    final workspace = container.read(workspaceProvider);
    expect(workspace.activeModel.ref, model.ref);
    expect(workspace.activeModel.name, 'Claude Sonnet');
  });

  test('workspace drives a full turn through an ACP client', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('acp')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final wire = StreamChannelController<String>();
    final server = AcpServer(runtime, models: [model]);
    final serverDone = server.serveChannel(wire.local);
    final client = AcpClient.channel(wire.foreign);
    await client.connect();
    addTearDown(() => serverDone);
    addTearDown(store.close);
    addTearDown(client.close);

    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: client,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    // Keep a listener so Riverpod does not reset the notifier state while
    // the turn is in flight without any widget subscribing to it.
    container.listen(workspaceProvider, (_, _) {});

    await controller.send('hello');

    final state = container.read(workspaceProvider);
    expect(state.messages.map((message) => message.kind), [
      WorkspaceMessageKind.user,
      WorkspaceMessageKind.reasoning,
      WorkspaceMessageKind.assistant,
    ]);
    expect(state.messages[1].text, 'first thought');
    expect(state.messages[2].text, 'Hello from Atlas.');
    expect(state.sessionId, isNotNull);
    expect(state.active.busy, isFalse);
    expect(state.active.turnPhase, TurnPhase.idle);
  });

  test('workspace lists sessions created through the ACP client', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('acp')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final wire = StreamChannelController<String>();
    final server = AcpServer(runtime, models: [model]);
    final serverDone = server.serveChannel(wire.local);
    final client = AcpClient.channel(wire.foreign);
    await client.connect();
    addTearDown(() => serverDone);
    addTearDown(store.close);
    addTearDown(client.close);

    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: client,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    container.listen(workspaceProvider, (_, _) {});

    await controller.send('hello');
    await controller.refreshSessions();

    final sessions = container.read(workspaceProvider).sessions;
    expect(sessions, hasLength(1));
    expect(sessions.first.workingDirectory, '/tmp');
  });

  test('workspace renameSession persists through the ACP client', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('acp')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final wire = StreamChannelController<String>();
    final server = AcpServer(runtime, models: [model]);
    final serverDone = server.serveChannel(wire.local);
    final client = AcpClient.channel(wire.foreign);
    await client.connect();
    addTearDown(() => serverDone);
    addTearDown(store.close);
    addTearDown(client.close);

    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: client,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    container.listen(workspaceProvider, (_, _) {});

    await controller.send('hello');
    final sessionId = container.read(workspaceProvider).sessionId!;
    await controller.renameSession(sessionId, 'Renamed title');

    final sessions = container.read(workspaceProvider).sessions;
    expect(sessions.single.title, 'Renamed title');
    expect(
      container
          .read(workspaceProvider)
          .messages
          .where((message) => message.kind == WorkspaceMessageKind.error),
      isEmpty,
    );
  });
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
    yield const TextDeltaEvent('Hello from Atlas.');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
        reasoning: 'first thought',
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

final class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.directory);

  final String directory;

  @override
  String build() => directory;
}

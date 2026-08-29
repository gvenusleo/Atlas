import 'dart:convert';
import 'dart:io';

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

import 'workspace_support.dart';

void main() {
  test('activateConnection switches the runtime and resets sessions', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('local')),
    );
    final store = DriftSessionStore.inMemory();
    final localRuntime = AgentRuntime(
      store: store,
      provider: FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final localEnv = RuntimeEnvironment(
      runtime: localRuntime,
      models: [model],
      skills: EmptySkillCatalog(),
    );
    final wire = StreamChannelController<String>();
    final acpServer = AcpServer(localRuntime, models: [model]);
    final serverDone = acpServer.serveChannel(wire.local);
    final acpClient = AcpClient.channel(wire.foreign);
    await acpClient.connect();

    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(local: localEnv),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(() => serverDone);
    addTearDown(acpClient.close);
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    container.listen(workspaceProvider, (_, _) {});

    await controller.send('hello');
    expect(container.read(workspaceProvider).sessionId, isNotNull);

    // Switch to an ACP connection: sessions reset and the new runtime is used.
    final runtimeController = container.read(
      runtimeEnvironmentProvider.notifier,
    );
    // Activate the already-connected client by swapping the environment
    // through the controller's test surface; bootstrapAcpClient would
    // spawn a real process.
    runtimeController.overrideEnvironmentForTest(acpClient);

    // The switch replaces the runtime environment.
    final runtimeState = container.read(runtimeEnvironmentProvider);
    expect(runtimeState.status, AcpConnectionStatus.connected);
    expect(runtimeState.environment, isNotNull);

    // Sessions reset to a fresh draft under the new runtime.
    final workspace = container.read(workspaceProvider);
    expect(workspace.messages, isEmpty);

    // The session list reloads from the ACP server after activation.
    await pumpEventQueue();
    final after = container.read(workspaceProvider);
    expect(after.sessions, hasLength(1));
    expect(after.sessions.first.workingDirectory, '/tmp');
  });
  test(
    'deactivateConnection restores the local runtime without closing it',
    () async {
      final model = ModelDescriptor(
        ref: ModelRef(
          providerId: ProviderId('test'),
          modelId: ModelId('local'),
        ),
      );
      final store = DriftSessionStore.inMemory();
      final localRuntime = AgentRuntime(
        store: store,
        provider: FakeProvider(model.ref),
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: model.ref,
      );
      final localEnv = RuntimeEnvironment(
        runtime: localRuntime,
        models: [model],
        skills: EmptySkillCatalog(),
      );
      final wire = StreamChannelController<String>();
      final acpServer = AcpServer(localRuntime, models: [model]);
      final serverDone = acpServer.serveChannel(wire.local);
      final acpClient = AcpClient.channel(wire.foreign);
      await acpClient.connect();

      final runtimeController = RuntimeEnvironmentController(local: localEnv);
      final container = ProviderContainer(
        overrides: [
          runtimeEnvironmentProvider.overrideWith(() => runtimeController),
        ],
      );
      addTearDown(store.close);
      addTearDown(() => serverDone);
      addTearDown(acpClient.close);
      addTearDown(container.dispose);
      container.read(runtimeEnvironmentProvider);

      runtimeController.overrideEnvironmentForTest(acpClient);
      expect(
        container.read(runtimeEnvironmentProvider).status,
        AcpConnectionStatus.connected,
      );

      await runtimeController.deactivateConnection();
      final restored = container.read(runtimeEnvironmentProvider);
      expect(restored.status, AcpConnectionStatus.disconnected);
      expect(identical(restored.environment, localEnv), isTrue);

      final session = await localRuntime.createSession(
        workingDirectory: '/tmp',
      );
      expect(session.workingDirectory, '/tmp');
    },
  );
  test('surfaces permission requests and forwards the reply', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('local')),
    );
    final store = DriftSessionStore.inMemory();
    final localRuntime = AgentRuntime(
      store: store,
      provider: FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final localEnv = RuntimeEnvironment(
      runtime: localRuntime,
      models: [model],
      skills: EmptySkillCatalog(),
    );
    final wire = StreamChannelController<String>();
    final acpServer = AcpServer(localRuntime, models: [model]);
    final serverDone = acpServer.serveChannel(wire.local);
    final acpClient = AcpClient.channel(wire.foreign);
    await acpClient.connect();

    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(local: localEnv),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(() => serverDone);
    addTearDown(acpClient.close);
    addTearDown(container.dispose);
    final controller = container.read(workspaceProvider.notifier);
    container.listen(workspaceProvider, (_, _) {});

    container
        .read(runtimeEnvironmentProvider.notifier)
        .overrideEnvironmentForTest(acpClient);
    await pumpEventQueue();

    // The server pushes a permission request for a tool call.
    wire.local.sink.add(
      jsonEncode({
        'jsonrpc': '2.0',
        'id': 999,
        'method': 'session/request_permission',
        'params': {
          'sessionId': 'sess-1',
          'toolCall': {
            'toolCallId': 'call-9',
            'toolName': 'edit',
            'title': 'Edit: /tmp/a.txt',
            'input': {'path': '/tmp/a.txt'},
          },
          'options': [
            {'optionId': 'once', 'kind': 'allow_once', 'name': 'Allow once'},
            {
              'optionId': 'always',
              'kind': 'allow_always',
              'name': 'Always allow',
            },
            {'optionId': 'reject', 'kind': 'reject_once', 'name': 'Reject'},
          ],
        },
      }),
    );
    await pumpEventQueue();

    final workspace = container.read(workspaceProvider);
    expect(workspace.pendingPermissions, hasLength(1));
    final request = workspace.pendingPermissions.single;
    expect(request.toolName, 'Edit: /tmp/a.txt');
    expect(request.options, hasLength(3));

    await controller.respondPermission(
      request.requestId,
      PermissionReply.allowAlways,
    );
    await pumpEventQueue();

    expect(container.read(workspaceProvider).pendingPermissions, isEmpty);
  });
  test('defaults the workspace directory to the home directory', () {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    final home =
        Platform.environment['HOME'] ?? Platform.environment['USERPROFILE'];
    expect(
      container.read(workspaceWorkingDirectoryProvider),
      home == null || home.isEmpty ? Directory.current.path : home,
    );
  });
  test('switching the working directory starts a new draft', () async {
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
    await controller.send('keep this session');
    final firstId = container.read(workspaceProvider).sessionId;
    controller.newSession(workingDirectory: '/tmp2');

    final state = container.read(workspaceProvider);
    expect(state.workingDirectory, '/tmp2');
    expect(state.messages, isEmpty);
    expect(state.sessionId, isNull);
    expect(firstId, isNotNull);
    expect(
      state.workspaces.values.any(
        (workspace) => workspace.sessionId == firstId,
      ),
      isTrue,
    );
  });
  test(
    'newSession is a no-op on an empty draft in the same directory',
    () async {
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
      final firstKey = container.read(workspaceProvider).activeKey;
      controller.newSession();
      expect(container.read(workspaceProvider).activeKey, firstKey);
      expect(container.read(workspaceProvider).workspaces, hasLength(1));
    },
  );
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
    await controller.send('first session');
    controller.newSession(workingDirectory: '/tmp2');
    await controller.send('second session');

    await controller.refreshSessions();
    final sessions = container.read(workspaceProvider).sessions;
    expect(sessions, hasLength(2));
    expect(sessions.map((session) => session.workingDirectory).toSet(), {
      '/tmp',
      '/tmp2',
    });
  });
  test('renameSession updates the title and refreshes the list', () async {
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
    await controller.send('first session');
    final sessionId = container.read(workspaceProvider).sessionId!;

    await controller.renameSession(sessionId, 'Renamed title');
    final sessions = container.read(workspaceProvider).sessions;
    expect(sessions.single.title, 'Renamed title');
  });
  test('selectMode applies immediately to an existing session', () async {
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
    final modeRuntime = FakeModeRuntime(runtime);
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: modeRuntime,
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
    await controller.send('first session');
    final sessionId = container.read(workspaceProvider).sessionId!;

    await controller.selectMode('plan');
    expect(container.read(workspaceProvider).mode, 'plan');
    expect(modeRuntime.modeCalls, [(sessionId.value, 'plan')]);
  });
  test('selectMode on a draft carries the mode into the first turn', () async {
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
    final modeRuntime = FakeModeRuntime(runtime);
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: modeRuntime,
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
    // Draft: no session yet, so selectMode only records the selection.
    await controller.selectMode('plan');
    expect(container.read(workspaceProvider).mode, 'plan');
    expect(modeRuntime.modeCalls, isEmpty);

    // The first turn creates the session and carries the mode in the request.
    await controller.send('hello');
    expect(modeRuntime.turnModes, ['plan']);
  });
  test('deleteSession removes the session and resets the active one', () async {
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
    await controller.send('first session');
    final sessionId = container.read(workspaceProvider).sessionId!;

    await controller.deleteSession(sessionId);
    final state = container.read(workspaceProvider);
    expect(state.sessions, isEmpty);
    expect(state.sessionId, isNull);
    expect(state.messages, isEmpty);
  });
  test(
    'a running session stays cached while another session is focused',
    () async {
      final model = ModelDescriptor(
        ref: ModelRef(
          providerId: ProviderId('test'),
          modelId: ModelId('streaming'),
        ),
        name: 'Streaming test model',
        reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
      );
      final store = DriftSessionStore.inMemory();
      final provider = BlockingProvider(model.ref);
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
      final firstTurn = controller.send('first session');
      await provider.firstStarted.future;
      final firstId = container.read(workspaceProvider).sessionId;
      expect(container.read(workspaceProvider).busy, isTrue);
      expect(container.read(workspaceProvider).runningSessionIds, {firstId});

      controller.newSession();
      expect(container.read(workspaceProvider).busy, isFalse);
      expect(container.read(workspaceProvider).messages, isEmpty);
      expect(container.read(workspaceProvider).runningSessionIds, {firstId});

      provider.releaseFirst.complete();
      await firstTurn;
      expect(container.read(workspaceProvider).runningSessionIds, isEmpty);
      expect(container.read(workspaceProvider).completedSessionIds, {firstId});
      await controller.resume(firstId!);
      expect(container.read(workspaceProvider).completedSessionIds, isEmpty);
      expect(
        container
            .read(workspaceProvider)
            .messages
            .map((message) => message.kind),
        [WorkspaceMessageKind.user, WorkspaceMessageKind.assistant],
      );
      expect(
        container.read(workspaceProvider).messages.last.text,
        'Hello from Atlas.',
      );
    },
  );
  test('model and reasoning effort stay with their session', () async {
    final modelA = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('a')),
      name: 'Model A',
      reasoningEfforts: const [
        ReasoningEffortOption(value: 'low'),
        ReasoningEffortOption(value: 'high'),
      ],
    );
    final modelB = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('b')),
      name: 'Model B',
      reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
    );
    final store = DriftSessionStore.inMemory();
    final provider = RecordingProvider();
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: modelA.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
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
    addTearDown(store.close);
    addTearDown(container.dispose);

    final controller = container.read(workspaceProvider.notifier);
    controller.selectReasoningEffort('high');
    await controller.send('first session');
    final firstId = container.read(workspaceProvider).sessionId!;
    expect(provider.models, [modelA.ref]);
    expect(provider.efforts, ['high']);

    controller.newSession();
    expect(container.read(workspaceProvider).activeModel.ref, modelA.ref);
    expect(container.read(workspaceProvider).reasoningEffort, 'high');
    controller.selectModel(modelB);
    expect(container.read(workspaceProvider).reasoningEffort, 'balanced');
    await controller.send('second session');
    expect(provider.models, [modelA.ref, modelB.ref]);
    expect(provider.efforts, ['high', 'balanced']);

    await controller.resume(firstId);
    expect(container.read(workspaceProvider).activeModel.ref, modelA.ref);
    expect(container.read(workspaceProvider).reasoningEffort, 'high');
    await controller.send('first again');
    expect(provider.models.last, modelA.ref);
    expect(provider.efforts.last, 'high');
  });
  test('tools sidebar tab stays with its session', () async {
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
    await controller.send('first session');
    final firstId = container.read(workspaceProvider).sessionId!;
    controller.setShowTerminal(true);
    expect(container.read(workspaceProvider).showTerminal, isTrue);

    controller.newSession();
    expect(container.read(workspaceProvider).showTerminal, isFalse);
    await controller.resume(firstId);
    expect(container.read(workspaceProvider).showTerminal, isTrue);
  });
}

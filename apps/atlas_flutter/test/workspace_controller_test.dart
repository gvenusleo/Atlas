import 'dart:async';
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
import 'package:atlas_flutter/features/workspace/application/workspace_state.dart';

void main() {
  test('activateConnection switches the runtime and resets sessions', () async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('local')),
    );
    final store = DriftSessionStore.inMemory();
    final localRuntime = AgentRuntime(
      store: store,
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final localEnv = RuntimeEnvironment(
      runtime: localRuntime,
      models: [model],
      skills: _EmptySkillCatalog(),
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
          () => _FixedWorkingDirectory('/tmp'),
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
        provider: _FakeProvider(model.ref),
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: model.ref,
      );
      final localEnv = RuntimeEnvironment(
        runtime: localRuntime,
        models: [model],
        skills: _EmptySkillCatalog(),
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
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final localEnv = RuntimeEnvironment(
      runtime: localRuntime,
      models: [model],
      skills: _EmptySkillCatalog(),
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
          () => _FixedWorkingDirectory('/tmp'),
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
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
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
    final provider = _GatedFakeProvider(model.ref);
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
              skills: _EmptySkillCatalog(),
            ),
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
      provider: _FakeProvider(model.ref),
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
              skills: _EmptySkillCatalog(),
            ),
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
      provider: _FakeProvider(model.ref),
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
              skills: _EmptySkillCatalog(),
            ),
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
        provider: _FakeProvider(model.ref),
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
                skills: _EmptySkillCatalog(),
              ),
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
      provider: _FakeProvider(model.ref),
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
              skills: _EmptySkillCatalog(),
            ),
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
      provider: _FakeProvider(model.ref),
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
              skills: _EmptySkillCatalog(),
            ),
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
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final modeRuntime = _FakeModeRuntime(runtime);
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: modeRuntime,
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
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final modeRuntime = _FakeModeRuntime(runtime);
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: modeRuntime,
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
      provider: _FakeProvider(model.ref),
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
              skills: _EmptySkillCatalog(),
            ),
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
    final sessionId = container.read(workspaceProvider).sessionId!;

    await controller.deleteSession(sessionId);
    final state = container.read(workspaceProvider);
    expect(state.sessions, isEmpty);
    expect(state.sessionId, isNull);
    expect(state.messages, isEmpty);
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
        provider: _FakeProvider(visionModel.ref),
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
                skills: _EmptySkillCatalog(),
              ),
            ),
          ),
          workspaceWorkingDirectoryProvider.overrideWith(
            () => _FixedWorkingDirectory('/tmp'),
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
      provider: _FakeProvider(visionModel.ref),
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
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
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

  test('send attaches images to the user turn', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final store = DriftSessionStore.inMemory();
    final provider = _RecordingProvider();
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
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
              models: [visionModel],
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    const image = ImageContent(
      source: 'data:image/png;base64,AAAA',
      mimeType: 'image/png',
    );
    final controller = container.read(workspaceProvider.notifier);
    final sent = await controller.send('describe this', images: const [image]);
    expect(sent, isTrue);

    final state = container.read(workspaceProvider);
    expect(state.hasImages, isTrue);
    expect(state.messages.first.kind, WorkspaceMessageKind.user);
    expect(state.messages.first.text, 'describe this');
    expect(state.messages.first.imageSources, [image.source]);
    expect(
      provider.contents.first.whereType<ImageContent>().single.source,
      image.source,
    );
  });

  test('send rejects images for slash commands', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(visionModel.ref),
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
              models: [visionModel],
              skills: _EmptySkillCatalog(),
            ),
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
    final sent = await controller.send(
      '/compact',
      images: const [ImageContent(source: 'data:image/png;base64,AAAA')],
    );
    expect(sent, isFalse);
    final state = container.read(workspaceProvider);
    expect(state.busy, isFalse);
    expect(state.messages.single.kind, WorkspaceMessageKind.notice);
    expect(state.messages.single.text, contains('do not support images'));
  });

  test('send rejects images when the model cannot accept them', () async {
    final textModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('text')),
      name: 'Text only',
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(textModel.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: textModel.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [textModel],
              skills: _EmptySkillCatalog(),
            ),
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
    final sent = await controller.send(
      'look',
      images: const [ImageContent(source: 'data:image/png;base64,AAAA')],
    );
    expect(sent, isFalse);
    expect(
      container.read(workspaceProvider).messages.single.text,
      contains('does not support image input'),
    );
  });

  test('resume restores user image thumbnails', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(visionModel.ref),
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
              models: [visionModel],
              skills: _EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    const image = ImageContent(
      source: 'data:image/png;base64,AAAA',
      mimeType: 'image/png',
    );
    final session = await runtime.createSession(workingDirectory: '/tmp');
    await runtime
        .run(
          TurnRequest(
            sessionId: session.id,
            content: const [TextContent('describe this'), image],
            workingDirectory: '/tmp',
            model: visionModel.ref,
          ),
        )
        .toList();

    await container.read(workspaceProvider.notifier).resume(session.id);
    final user = container
        .read(workspaceProvider)
        .messages
        .firstWhere((message) => message.kind == WorkspaceMessageKind.user);
    expect(user.text, 'describe this');
    expect(user.imageSources, [image.source]);
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
      final provider = _BlockingProvider(model.ref);
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
                skills: _EmptySkillCatalog(),
              ),
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
    final provider = _RecordingProvider();
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
              skills: _EmptySkillCatalog(),
            ),
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
      provider: _FakeProvider(model.ref),
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
              skills: _EmptySkillCatalog(),
            ),
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
    final firstId = container.read(workspaceProvider).sessionId!;
    controller.setShowTerminal(true);
    expect(container.read(workspaceProvider).showTerminal, isTrue);

    controller.newSession();
    expect(container.read(workspaceProvider).showTerminal, isFalse);
    await controller.resume(firstId);
    expect(container.read(workspaceProvider).showTerminal, isTrue);
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
        provider: _RecordingProvider(),
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
                skills: _EmptySkillCatalog(),
              ),
            ),
          ),
          workspaceWorkingDirectoryProvider.overrideWith(
            () => _FixedWorkingDirectory('/tmp'),
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
        provider: _RecordingProvider(),
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
                skills: _EmptySkillCatalog(),
              ),
            ),
          ),
          workspaceWorkingDirectoryProvider.overrideWith(
            () => _FixedWorkingDirectory('/tmp'),
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

/// Working directory fixed for tests.
final class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

final class _GatedFakeProvider implements ModelProvider {
  _GatedFakeProvider(this.model);

  final ModelRef model;
  final reasoningGate = Completer<void>();
  final textGate = Completer<void>();

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    expect(request.model, model);
    yield const ReasoningDeltaEvent('first thought');
    await reasoningGate.future;
    yield const TextDeltaEvent('Hello from Atlas.');
    await textGate.future;
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
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

/// A runtime that advertises agent modes and records mode switches.
final class _FakeModeRuntime implements RuntimeService {
  _FakeModeRuntime(this._inner);

  final AgentRuntime _inner;
  final modeCalls = <(String, String)>[];
  final turnModes = <String?>[];

  static const modes = [
    ModeOption(id: 'build', name: 'build'),
    ModeOption(id: 'plan', name: 'plan'),
  ];

  @override
  ModelRef get defaultModel => _inner.defaultModel;

  @override
  Stream<AgentEvent> run(TurnRequest request) {
    turnModes.add(request.mode);
    return _inner.run(request);
  }

  @override
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    CancellationToken? cancellation,
  }) => _inner.compact(
    sessionId,
    instruction: instruction,
    cancellation: cancellation,
  );

  @override
  Future<SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  }) => _inner.listSessions(
    workingDirectory: workingDirectory,
    cursor: cursor,
    limit: limit,
  );

  @override
  Future<Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const <String>[],
  }) => _inner.createSession(
    workingDirectory: workingDirectory,
    additionalDirectories: additionalDirectories,
  );

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      _inner.loadSession(sessionId);

  @override
  Future<void> deleteSession(SessionId sessionId) =>
      _inner.deleteSession(sessionId);

  @override
  Future<void> renameSession(SessionId sessionId, String title) =>
      _inner.renameSession(sessionId, title);

  @override
  Future<int> contextWindowSize() => _inner.contextWindowSize();

  @override
  String? titleFor(SessionId sessionId) => _inner.titleFor(sessionId);

  @override
  List<AgentCommand> commandsFor(SessionId sessionId) =>
      _inner.commandsFor(sessionId);

  @override
  List<ModeOption> get modeOptions => modes;

  @override
  String? modeFor(SessionId sessionId) => null;

  @override
  Future<void> setMode(SessionId sessionId, String modeId) async {
    modeCalls.add((sessionId.value, modeId));
  }
}

final class _RecordingProvider implements ModelProvider {
  final models = <ModelRef>[];
  final efforts = <String?>[];
  final contents = <List<ContentPart>>[];

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async => ModelDescriptor(
    ref: requested,
    inputCapabilities: const {
      ModelInputCapability.text,
      ModelInputCapability.image,
    },
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    models.add(request.model);
    efforts.add(request.reasoningEffort);
    contents.add(request.messages.first.content);
    yield const TextDeltaEvent('ok');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('ok')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class _BlockingProvider implements ModelProvider {
  _BlockingProvider(this.model);

  final ModelRef model;
  final firstStarted = Completer<void>();
  final releaseFirst = Completer<void>();

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (!firstStarted.isCompleted) {
      firstStarted.complete();
      await releaseFirst.future;
    }
    yield const TextDeltaEvent('Hello from Atlas.');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

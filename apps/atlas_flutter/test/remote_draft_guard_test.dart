import 'dart:io';

import 'package:atlas_flutter/app/remote_connections.dart';
import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:atlas_ws/atlas_ws.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'workspace_support.dart';

/// A remote profile without a directory must not start a session on the
/// first message: the workspace guard blocks drafts until the user picks the
/// computer-side directory, and the chosen directory reaches the server as
/// the session `cwd`.
void main() {
  late Directory temp;
  late RemoteTokenFile tokens;
  late AtlasWsServer server;
  late HttpServer http;
  late ProviderContainer container;
  late ModelDescriptor model;

  setUp(() async {
    temp = await Directory.systemTemp.createTemp('atlas_remote_guard_test');
    tokens = RemoteTokenFile(File('${temp.path}/token'));
    await tokens.loadOrCreate();
    model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('local')),
    );
    server = AtlasWsServer(
      runtime: _fullRuntime(model),
      models: [model],
      authorize: tokens.authorize,
      log: (_) {},
    );
    http = await server.start(port: 0);
    container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          RuntimeEnvironmentController.new,
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/before'),
        ),
      ],
    );
    addTearDown(container.dispose);
    addTearDown(() => server.stop());
    addTearDown(() => http.close(force: true));
    addTearDown(() => temp.delete(recursive: true));
  });

  Future<void> waitForConnected() async {
    final deadline = DateTime.now().add(const Duration(seconds: 15));
    while (DateTime.now().isBefore(deadline)) {
      final state = container.read(runtimeEnvironmentProvider);
      if (state.remoteStatus == RemoteConnectionStatus.connected) {
        return;
      }
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }
    fail(
      'connection did not succeed: '
      '${container.read(runtimeEnvironmentProvider).remoteError}',
    );
  }

  Future<void> connectWithoutDirectory() async {
    final token = await tokens.loadOrCreate();
    await container
        .read(runtimeEnvironmentProvider.notifier)
        .activateRemote(
          RemoteConnectionProfile(
            name: 'nodir',
            wsUrl: 'ws://127.0.0.1:${http.port}/acp',
            token: token,
          ),
        );
    await waitForConnected();
  }

  test('the first message is blocked until a directory is chosen', () async {
    await connectWithoutDirectory();
    _keepWorkspaceAlive(container);
    final sent = await container
        .read(workspaceProvider.notifier)
        .send('hello?');

    expect(sent, isFalse);
    final state = container.read(workspaceProvider);
    expect(state.sessionId, isNull);
    expect(state.messages.map((message) => message.kind), [
      WorkspaceMessageKind.notice,
    ]);
    expect(state.messages.single.text, contains('working directory'));
    // No session reached the server.
    final page = await container
        .read(runtimeEnvironmentProvider)
        .environment!
        .runtime
        .listSessions();
    expect(page.items, isEmpty);
  });

  test('the chosen directory flows into the created session', () async {
    await connectWithoutDirectory();
    container
        .read(runtimeEnvironmentProvider.notifier)
        .setRemoteWorkingDirectory('/remote-project');
    // The bar starts a fresh draft rooted at the chosen directory, exactly
    // like switching directories on the desktop.
    _keepWorkspaceAlive(container);
    container
        .read(workspaceProvider.notifier)
        .newSession(workingDirectory: '/remote-project');
    expect(
      container.read(workspaceWorkingDirectoryProvider),
      '/remote-project',
    );

    final sent = await container
        .read(workspaceProvider.notifier)
        .send('hello?');

    expect(sent, isTrue);
    final state = container.read(workspaceProvider);
    expect(state.sessionId, isNotNull);
    expect(state.messages.map((message) => message.kind), [
      WorkspaceMessageKind.user,
      WorkspaceMessageKind.reasoning,
      WorkspaceMessageKind.assistant,
    ]);
    final sessions = await container
        .read(runtimeEnvironmentProvider)
        .environment!
        .runtime
        .listSessions();
    expect(sessions.items, hasLength(1));
    expect(sessions.items.single.workingDirectory, '/remote-project');
  });
}

/// Keeps the autoDispose workspace provider alive across an awaited turn.
///
/// Real UIs hold a widget listener on [workspaceProvider]; headless tests
/// must register one, otherwise the provider is disposed during the
/// event-loop gaps of a streaming remote turn.
void _keepWorkspaceAlive(ProviderContainer container) {
  // The subscription lives until the container is disposed in tearDown.
  container.listen(workspaceProvider, (previous, next) {});
}

/// A full in-process runtime with a responding model, so the guard test can
/// observe an actual turn completing over the WebSocket transport.
AgentRuntime _fullRuntime(ModelDescriptor model) => AgentRuntime(
  store: DriftSessionStore.inMemory(),
  provider: FakeProvider(model.ref),
  tools: LocalToolRegistry(const []),
  ids: SecureIdGenerator(),
  defaultModel: model.ref,
);

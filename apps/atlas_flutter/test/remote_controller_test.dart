import 'dart:async';
import 'dart:io';

import 'package:atlas_flutter/app/remote_connections.dart';
import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'workspace_support.dart';
import 'package:atlas_ws/atlas_ws.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory temp;
  late RemoteTokenFile tokens;
  late AtlasWsServer server;
  late HttpServer http;
  late ProviderContainer container;

  setUp(() async {
    temp = await Directory.systemTemp.createTemp('atlas_remote_test');
    tokens = RemoteTokenFile(File('${temp.path}/token'));
    await tokens.loadOrCreate();
    server = AtlasWsServer(
      runtime: _testRuntime(),
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

  Future<void> connectProfile() async {
    final token = await tokens.loadOrCreate();
    await _controller(container).activateRemote(
      RemoteConnectionProfile(
        name: 'test',
        wsUrl: 'ws://127.0.0.1:${http.port}/acp',
        token: token,
        workingDirectory: '/',
      ),
    );
  }

  Future<void> waitFor(
    bool Function(AcpRuntimeState) predicate, [
    Duration timeout = const Duration(seconds: 15),
  ]) async {
    final deadline = DateTime.now().add(timeout);
    while (DateTime.now().isBefore(deadline)) {
      final state = container.read(runtimeEnvironmentProvider);
      if (predicate(state)) {
        return;
      }
      await Future<void>.delayed(const Duration(milliseconds: 100));
    }
    fail(
      'state condition not met within $timeout: '
      '${container.read(runtimeEnvironmentProvider).remoteError}',
    );
  }

  test('activateRemote connects and marks the environment remote', () async {
    await connectProfile();
    final state = container.read(runtimeEnvironmentProvider);
    expect(state.remoteStatus, RemoteConnectionStatus.connected);
    expect(state.environment, isNotNull);
    expect(state.environment!.isRemote, isTrue);
  });

  test('a bad token surfaces an error state and throws', () async {
    await expectLater(
      _controller(container).activateRemote(
        RemoteConnectionProfile(
          name: 'bad',
          wsUrl: 'ws://127.0.0.1:${http.port}/acp',
          token: 'wrong',
          workingDirectory: '/unreachable',
        ),
      ),
      throwsStateError,
    );
    final state = container.read(runtimeEnvironmentProvider);
    expect(state.remoteStatus, RemoteConnectionStatus.error);
    expect(state.remoteProfile, isNotNull);
    expect(state.environment, isNull);
    // A failed attempt must not repoint the working directory.
    expect(container.read(workspaceWorkingDirectoryProvider), '/before');
  });

  test(
    'a successful connection applies the profile working directory',
    () async {
      final token = await tokens.loadOrCreate();
      await _controller(container).activateRemote(
        RemoteConnectionProfile(
          name: 'wd',
          wsUrl: 'ws://127.0.0.1:${http.port}/acp',
          token: token,
          workingDirectory: '/on-the-computer',
        ),
      );
      expect(
        container.read(workspaceWorkingDirectoryProvider),
        '/on-the-computer',
      );
    },
  );

  test(
    'a profile without a directory never touches the working directory',
    () async {
      final token = await tokens.loadOrCreate();
      await _controller(container).activateRemote(
        RemoteConnectionProfile(
          name: 'nodir',
          wsUrl: 'ws://127.0.0.1:${http.port}/acp',
          token: token,
        ),
      );
      final state = container.read(runtimeEnvironmentProvider);
      expect(state.remoteStatus, RemoteConnectionStatus.connected);
      expect(state.remoteProfile!.workingDirectory, isNull);
      expect(container.read(workspaceWorkingDirectoryProvider), '/before');
    },
  );

  test(
    'setRemoteWorkingDirectory updates the profile and the directory',
    () async {
      final token = await tokens.loadOrCreate();
      await _controller(container).activateRemote(
        RemoteConnectionProfile(
          name: 'nodir',
          wsUrl: 'ws://127.0.0.1:${http.port}/acp',
          token: token,
        ),
      );
      _controller(container).setRemoteWorkingDirectory('/projects/alpha');
      final state = container.read(runtimeEnvironmentProvider);
      expect(state.remoteProfile!.workingDirectory, '/projects/alpha');
      expect(
        container.read(workspaceWorkingDirectoryProvider),
        '/projects/alpha',
      );
    },
  );

  test('the workspace provider stays readable before and after a remote '
      'connection', () async {
    // Mobile clients mount the workspace shell with no runtime (remote
    // connection screen); the provider must render an idle draft instead of
    // entering an unrecoverable error state.
    // The autoDispose workspace provider needs a listener to survive the
    // awaited remote connection, mirroring real UI watchers.
    container.listen(workspaceProvider, (previous, next) {});
    final idle = container.read(workspaceProvider);
    expect(idle.sessionId, isNull);
    expect(idle.workspaces[idle.activeKey], isNotNull);

    await connectProfile();
    // Reading again must not throw: the runtime switch resets the workspace
    // into a live draft backed by the remote runtime.
    final live = container.read(workspaceProvider);
    expect(live.sessionId, isNull);
    expect(live.workspaces[live.activeKey], isNotNull);
  });

  test('disconnectRemote restores an empty environment', () async {
    await connectProfile();
    await _controller(container).disconnectRemote();
    final state = container.read(runtimeEnvironmentProvider);
    expect(state.remoteProfile, isNull);
    expect(state.remoteStatus, RemoteConnectionStatus.disconnected);
    expect(state.environment, isNull);
  });

  test('a dropped connection enters reconnecting and recovers', () async {
    await connectProfile();
    final port = http.port;
    // Kill the server: the client observes the socket close.
    await server.stop();
    await http.close(force: true);
    await waitFor(
      (state) => state.remoteStatus == RemoteConnectionStatus.reconnecting,
      const Duration(seconds: 8),
    );

    // Restart on the same port; the backoff loop reconnects automatically.
    server = AtlasWsServer(
      runtime: _testRuntime(),
      authorize: tokens.authorize,
      log: (_) {},
    );
    http = await server.start(port: port);
    await waitFor(
      (state) => state.remoteStatus == RemoteConnectionStatus.connected,
      const Duration(seconds: 20),
    );
    final state = container.read(runtimeEnvironmentProvider);
    expect(state.environment!.isRemote, isTrue);
  });
}

/// Minimal runtime store that supports session creation only: the remote
/// bootstrap probes one session and discards it, and no turns are run here.
AgentRuntime _testRuntime() => AgentRuntime(
  store: _CreateOnlyStore(),
  provider: _UnusedProvider(),
  tools: _NoTools(),
  ids: _TestIds(),
  defaultModel: ModelRef(
    providerId: ProviderId('test'),
    modelId: ModelId('model'),
  ),
);

final class _TestIds implements IdGenerator {
  var _count = 0;

  @override
  SessionId sessionId() => SessionId('session-${++_count}');

  @override
  TurnId turnId() => TurnId('turn-${++_count}');

  @override
  TimelineItemId timelineItemId() => TimelineItemId('item-${++_count}');
}

final class _CreateOnlyStore implements SessionStore {
  @override
  Future<void> createSession(Session session) async {}

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      throw UnsupportedError('not used by remote controller tests');

  @override
  Future<SessionPage> listSessions(SessionQuery query) =>
      throw UnsupportedError('not used by remote controller tests');

  @override
  Future<void> beginTurn(BeginTurn operation) =>
      throw UnsupportedError('not used by remote controller tests');

  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) => throw UnsupportedError('not used by remote controller tests');

  @override
  Future<void> appendToolResult(SessionId sessionId, ToolResultItem item) =>
      throw UnsupportedError('not used by remote controller tests');

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) =>
      throw UnsupportedError('not used by remote controller tests');

  @override
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  ) => throw UnsupportedError('not used by remote controller tests');

  @override
  Future<void> deleteSession(SessionId sessionId) async {}

  @override
  Future<void> renameSession(SessionId sessionId, String title) async {}
}

final class _UnusedProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) =>
      throw UnsupportedError('not used by remote controller tests');

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) =>
      throw UnsupportedError('not used by remote controller tests');
}

final class _NoTools implements ToolRegistry {
  @override
  List<ToolDescriptor> get descriptors => const [];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) =>
      throw UnsupportedError('not used by remote controller tests');
}

RuntimeEnvironmentController _controller(ProviderContainer container) =>
    container.read(runtimeEnvironmentProvider.notifier);

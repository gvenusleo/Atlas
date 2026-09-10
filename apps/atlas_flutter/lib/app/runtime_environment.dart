import 'dart:async';
import 'dart:io';
import 'dart:math' as math;

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'acp_bootstrap.dart';
import 'acp_connections.dart';
import 'remote_bootstrap.dart';
import 'remote_connections.dart';
import '../../features/workspace/application/workspace_controller.dart';

/// Runtime services and catalogs injected into Flutter presentation code.
final class RuntimeEnvironment {
  /// Creates an environment around the shared agent runtime.
  const RuntimeEnvironment({
    required this.runtime,
    required this.models,
    required this.skills,
    this.onClose,
    this.isRemote = false,
    this.closed,
  });

  /// The single runtime used by every Flutter feature.
  final PresentationAgentSession runtime;

  /// Models configured for user selection.
  final List<ModelDescriptor> models;

  /// Skills available to slash-command completion.
  final SkillCatalog skills;

  /// Closes process-owned resources when the application exits.
  final Future<void> Function()? onClose;

  /// Whether the runtime is served by a remote `atlas server` over
  /// WebSocket; remote sessions cannot browse or run local terminals.
  final bool isRemote;

  /// Completes when the underlying connection ends, for remote runtimes.
  final Future<void>? closed;

  /// Releases resources owned by this application environment.
  Future<void> close() async => onClose?.call();
}

/// Result of loading the local Atlas configuration and composing the runtime.
final class RuntimeBootstrap {
  /// Creates a successful bootstrap result.
  const RuntimeBootstrap.ready(this.environment) : error = null;

  /// Creates a failed bootstrap result that keeps the application usable.
  const RuntimeBootstrap.failed(this.error) : environment = null;

  /// The composed runtime environment when configuration succeeded.
  final RuntimeEnvironment? environment;

  /// A user-visible startup failure when configuration did not succeed.
  final String? error;
}

/// Supplies the composed runtime to workspace presentation code.
final runtimeEnvironmentProvider =
    NotifierProvider<RuntimeEnvironmentController, AcpRuntimeState>(
      RuntimeEnvironmentController.new,
    );

/// The lifecycle state of a local ACP subprocess connection.
enum AcpConnectionStatus {
  /// No ACP connection is active; the local runtime is in use.
  disconnected,

  /// An ACP server process is starting.
  connecting,

  /// An ACP server is active.
  connected,

  /// The last activation attempt failed.
  error,
}

/// The lifecycle state of a remote WebSocket connection.
enum RemoteConnectionStatus {
  /// No remote connection is active.
  disconnected,

  /// The first connection attempt is running.
  connecting,

  /// The remote connection is active.
  connected,

  /// The connection dropped and automatic reconnection is in progress.
  reconnecting,

  /// The last connection attempt failed and the profile stays editable.
  error,
}

/// The full runtime state exposed by [RuntimeEnvironmentController].
final class AcpRuntimeState {
  /// Creates a runtime state.
  const AcpRuntimeState({
    required this.environment,
    this.status = AcpConnectionStatus.disconnected,
    this.activationError,
    this.activeConnection,
    this.remoteProfile,
    this.remoteStatus = RemoteConnectionStatus.disconnected,
    this.remoteError,
  });

  /// The active runtime environment, or null on startup failure.
  final RuntimeEnvironment? environment;

  /// The local ACP connection lifecycle state.
  final AcpConnectionStatus status;

  /// The last local ACP activation error, if any.
  final String? activationError;

  /// The local ACP connection currently in use, or null for the local
  /// runtime.
  final AcpConnection? activeConnection;

  /// The remote profile currently in use, or null when none is active.
  final RemoteConnectionProfile? remoteProfile;

  /// The remote WebSocket connection lifecycle state.
  final RemoteConnectionStatus remoteStatus;

  /// The last remote connection error, if any.
  final String? remoteError;
}

/// Controls the active runtime, switching between the local runtime, ACP
/// subprocess connections, and remote WebSocket connections.
final class RuntimeEnvironmentController extends Notifier<AcpRuntimeState> {
  /// Creates a controller around the startup [local] runtime.
  RuntimeEnvironmentController({this._local});

  /// The locally composed runtime built at startup; null on mobile clients.
  final RuntimeEnvironment? _local;

  RemoteSessionHandle? _remoteHandle;
  var _remoteGeneration = 0;
  var _disconnectRequested = false;
  var _disposed = false;

  @override
  AcpRuntimeState build() {
    final local = _local;
    ref.onDispose(() {
      _disposed = true;
      unawaited(local?.close());
      unawaited(_remoteHandle?.environment.close());
    });
    return AcpRuntimeState(environment: _local);
  }

  /// Activates [connection], replacing the runtime with an ACP client.
  ///
  /// Sets the status to [AcpConnectionStatus.connected] or
  /// [AcpConnectionStatus.error]; the current runtime stays active when the
  /// server process cannot start.
  Future<void> activateConnection(AcpConnection connection) async {
    _dropRemote();
    state = AcpRuntimeState(
      environment: state.environment,
      status: AcpConnectionStatus.connecting,
    );
    final bootstrap = await bootstrapAcpClient(connection);
    final environment = bootstrap.environment;
    if (environment == null) {
      state = AcpRuntimeState(
        environment: state.environment,
        status: AcpConnectionStatus.error,
        activationError: bootstrap.error ?? 'Cannot start ACP server',
      );
      throw StateError(state.activationError!);
    }
    final previous = state.environment;
    state = AcpRuntimeState(
      environment: environment,
      status: AcpConnectionStatus.connected,
      activeConnection: connection,
    );
    // Keep the local in-process runtime alive so deactivate can restore it.
    if (!identical(previous, _local)) {
      await previous?.close();
    }
  }

  /// Switches back to the local runtime.
  Future<void> deactivateConnection() async {
    final local = _local;
    if (local == null) {
      return;
    }
    _dropRemote();
    final previous = state.environment;
    state = AcpRuntimeState(
      environment: local,
      status: AcpConnectionStatus.disconnected,
    );
    if (!identical(previous, local)) {
      await previous?.close();
    }
  }

  /// Drops the remote connection and restores the local runtime, or leaves
  /// the environment empty on mobile clients.
  Future<void> disconnectRemote() async {
    _disconnectRequested = true;
    _remoteGeneration++;
    final handle = _remoteHandle;
    _remoteHandle = null;
    final local = _local;
    state = AcpRuntimeState(
      environment: local,
      status: AcpConnectionStatus.disconnected,
    );
    if (handle != null && !identical(handle.environment, local)) {
      await handle.environment.close();
    }
  }

  /// Connects to the remote [profile], replacing the active runtime.
  ///
  /// The local runtime (when present) is kept alive so [disconnectRemote]
  /// can restore it. When the connection later drops, the controller
  /// reconnects with exponential backoff and swaps in a fresh client; the
  /// workspace resets its caches through its own listener on the provider.
  Future<void> activateRemote(RemoteConnectionProfile profile) async {
    final generation = ++_remoteGeneration;
    _disconnectRequested = false;
    state = AcpRuntimeState(
      environment: state.environment,
      status: AcpConnectionStatus.disconnected,
      remoteProfile: profile,
      remoteStatus: RemoteConnectionStatus.connecting,
    );
    try {
      final handle = await bootstrapRemoteConnection(profile);
      if (generation != _remoteGeneration) {
        await handle.environment.close();
        return;
      }
      _applyRemoteWorkingDirectory(profile);
      _adoptRemote(handle, profile);
      unawaited(_watchRemote(handle, profile, generation));
    } catch (error) {
      if (generation != _remoteGeneration) {
        return;
      }
      state = AcpRuntimeState(
        environment: state.environment,
        status: AcpConnectionStatus.disconnected,
        remoteProfile: profile,
        remoteStatus: RemoteConnectionStatus.error,
        remoteError: error.toString(),
      );
      throw StateError('Cannot connect to ${profile.wsUrl}: $error');
    }
  }

  /// Records the computer-side working directory on the active remote
  /// profile and points session drafts at it.
  ///
  /// No-op when no remote profile is active or [directory] is blank.
  /// Callers persist the updated profile through the connection store; this
  /// only updates the in-memory state so the UI and reconnection logic use
  /// the directory immediately.
  void setRemoteWorkingDirectory(String directory) {
    final trimmed = directory.trim();
    final profile = state.remoteProfile;
    if (profile == null || trimmed.isEmpty) {
      return;
    }
    final updated = profile.copyWith(workingDirectory: trimmed);
    state = AcpRuntimeState(
      environment: state.environment,
      status: state.status,
      activationError: state.activationError,
      activeConnection: state.activeConnection,
      remoteProfile: updated,
      remoteStatus: state.remoteStatus,
      remoteError: state.remoteError,
    );
    _applyRemoteWorkingDirectory(updated);
  }

  /// Invalidates any active remote connection state (generation, handle, and
  /// stale environment) before switching back to a local runtime path.
  void _dropRemote() {
    _disconnectRequested = true;
    _remoteGeneration++;
    final handle = _remoteHandle;
    _remoteHandle = null;
    final local = _local;
    if (handle != null && !identical(handle.environment, local)) {
      unawaited(handle.environment.close());
    }
  }

  /// Points session drafts at the computer-side directory of [profile].
  ///
  /// Applied only after a connection succeeds so a failed attempt never
  /// changes the working directory of subsequent local sessions.
  void _applyRemoteWorkingDirectory(RemoteConnectionProfile profile) {
    final workingDirectory = profile.workingDirectory;
    if (workingDirectory == null || workingDirectory.isEmpty) {
      return;
    }
    ref.read(workspaceWorkingDirectoryProvider.notifier).set(workingDirectory);
  }

  /// Marks the runtime state as offline while keeping the current
  /// environment visible, used when a remote connection drops.
  void _markOffline(RemoteConnectionProfile profile) {
    if (_disposed) {
      return;
    }
    state = AcpRuntimeState(
      environment: state.environment,
      status: AcpConnectionStatus.disconnected,
      remoteProfile: profile,
      remoteStatus: RemoteConnectionStatus.reconnecting,
    );
  }

  void _adoptRemote(
    RemoteSessionHandle handle,
    RemoteConnectionProfile profile,
  ) {
    final previous = state.environment;
    _remoteHandle = handle;
    state = AcpRuntimeState(
      environment: handle.environment,
      status: AcpConnectionStatus.connected,
      remoteProfile: profile,
      remoteStatus: RemoteConnectionStatus.connected,
    );
    if (!identical(previous, _local) &&
        !identical(previous, handle.environment)) {
      unawaited(previous?.close());
    }
  }

  /// Waits for [handle] to drop and schedules automatic reconnection.
  Future<void> _watchRemote(
    RemoteSessionHandle handle,
    RemoteConnectionProfile profile,
    int generation,
  ) async {
    await handle.closed;
    if (_disposed || generation != _remoteGeneration || _disconnectRequested) {
      return;
    }
    _markOffline(profile);
    await _reconnectLoop(profile, generation);
  }

  /// Reconnects with exponential backoff until the remote profile is
  /// replaced, disconnected, or a connection succeeds.
  Future<void> _reconnectLoop(
    RemoteConnectionProfile profile,
    int generation,
  ) async {
    var delay = const Duration(seconds: 1);
    const maximum = Duration(seconds: 30);
    while (generation == _remoteGeneration &&
        !_disconnectRequested &&
        !_disposed) {
      await Future<void>.delayed(delay);
      if (_disposed ||
          generation != _remoteGeneration ||
          _disconnectRequested) {
        return;
      }
      try {
        final handle = await bootstrapRemoteConnection(profile);
        if (generation != _remoteGeneration) {
          await handle.environment.close();
          return;
        }
        _applyRemoteWorkingDirectory(profile);
        _adoptRemote(handle, profile);
        unawaited(_watchRemote(handle, profile, generation));
        return;
      } on Object catch (error) {
        if (_disposed) {
          return;
        }
        delay = Duration(
          seconds: math.min(delay.inSeconds * 2, maximum.inSeconds),
        );
        if (state.remoteStatus == RemoteConnectionStatus.reconnecting) {
          // Keep the profile visible and record the last failure so the UI
          // can explain why reconnection has not succeeded yet.
          state = AcpRuntimeState(
            environment: state.environment,
            status: AcpConnectionStatus.disconnected,
            remoteProfile: profile,
            remoteStatus: RemoteConnectionStatus.reconnecting,
            remoteError: error.toString(),
          );
        }
      }
    }
  }

  /// Replaces the active environment with [environment], used by tests that
  /// cannot spawn a real ACP server process.
  @visibleForTesting
  void overrideEnvironmentForTest(PresentationAgentSession runtime) {
    final previous = state.environment;
    state = AcpRuntimeState(
      environment: RuntimeEnvironment(
        runtime: runtime,
        models: const [],
        skills: const NoopSkillCatalog(),
      ),
      status: AcpConnectionStatus.connected,
    );
    if (!identical(previous, _local)) {
      unawaited(previous?.close());
    }
  }
}

/// An empty skill catalog used for test and remote environments.
final class NoopSkillCatalog implements SkillCatalog {
  /// Creates an empty catalog.
  const NoopSkillCatalog();

  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}

/// Supplies a configuration or startup failure to the workspace.
final runtimeStartupErrorProvider = Provider<String?>((ref) => null);

/// Loads `~/.atlas/config.yaml` and composes the Flutter process runtime.
///
/// The local runtime is exposed through an in-process ACP server and consumed
/// through an [AcpClient], so presentation code always speaks ACP regardless
/// of whether the agent is local or remote. Mobile clients skip this and
/// start from the remote connection screen.
Future<RuntimeBootstrap> bootstrapRuntime({
  Map<String, String>? environment,
}) async {
  final values = environment ?? Platform.environment;
  final home = values['HOME'] ?? values['USERPROFILE'];
  if (home == null || home.isEmpty) {
    return const RuntimeBootstrap.failed(
      'Cannot locate the home directory for Atlas configuration.',
    );
  }

  final configFile = File('$home/.atlas/config.yaml');
  DriftSessionStore? store;
  try {
    final config = loadConfig(configFile);
    store = DriftSessionStore.openFile(File(config.session.dbPath));
    final runtime = composeRuntime(config, store: store);
    final models = List<ModelDescriptor>.unmodifiable(composeModels(config));
    // Expose the local runtime through an in-process ACP agent and consume it
    // as a client, so the Flutter app is always an ACP client.
    final server = AcpServer(runtime, models: models);
    final (serverDone, clientTransport) = server.serveMemory();
    final client = AcpClient(
      clientTransport,
      catalog: models,
      defaultModel: runtime.defaultModel,
    );
    await client.connect();
    return RuntimeBootstrap.ready(
      RuntimeEnvironment(
        runtime: client,
        models: client.catalog.isEmpty ? models : client.catalog,
        // Skills and slash commands come from the ACP agent session.
        skills: const NoopSkillCatalog(),
        onClose: () async {
          await client.close();
          await serverDone;
          await store?.close();
        },
      ),
    );
  } on ConfigLoadException catch (error) {
    store?.close();
    return RuntimeBootstrap.failed('Cannot load ${configFile.path}: $error');
  } on Object catch (error) {
    store?.close();
    return RuntimeBootstrap.failed('Cannot start Atlas: $error');
  }
}

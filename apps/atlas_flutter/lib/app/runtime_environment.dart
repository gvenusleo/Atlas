import 'dart:async';
import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'acp_bootstrap.dart';
import 'acp_connections.dart';

/// Runtime services and catalogs injected into Flutter presentation code.
final class RuntimeEnvironment {
  /// Creates an environment around the shared agent runtime.
  const RuntimeEnvironment({
    required this.runtime,
    required this.models,
    required this.skills,
    this.onClose,
  });

  /// The single runtime used by every Flutter feature.
  final RuntimeService runtime;

  /// Models configured for user selection.
  final List<ModelDescriptor> models;

  /// Skills available to slash-command completion.
  final SkillCatalog skills;

  /// Closes process-owned resources when the application exits.
  final Future<void> Function()? onClose;

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

/// The lifecycle state of the ACP client mode.
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

/// The full runtime state exposed by [RuntimeEnvironmentController].
final class AcpRuntimeState {
  /// Creates a runtime state.
  const AcpRuntimeState({
    required this.environment,
    this.status = AcpConnectionStatus.disconnected,
    this.activationError,
    this.activeConnection,
  });

  /// The active runtime environment, or null on startup failure.
  final RuntimeEnvironment? environment;

  /// The ACP connection lifecycle state.
  final AcpConnectionStatus status;

  /// The last activation error, or null when the last attempt succeeded.
  final String? activationError;

  /// The remote connection currently in use, or null for the local runtime.
  final AcpConnection? activeConnection;
}

/// Controls the active runtime, switching between the local runtime and ACP
/// server connections.
final class RuntimeEnvironmentController extends Notifier<AcpRuntimeState> {
  /// Creates a controller around the startup [local] runtime.
  RuntimeEnvironmentController({this._local});

  /// The locally composed runtime built at startup.
  final RuntimeEnvironment? _local;

  @override
  AcpRuntimeState build() {
    final local = _local;
    ref.onDispose(() {
      unawaited(local?.close());
    });
    return AcpRuntimeState(environment: _local);
  }

  /// Activates [connection], replacing the runtime with an ACP client.
  ///
  /// Sets the status to [AcpConnectionStatus.connected] or
  /// [AcpConnectionStatus.error]; the current runtime stays active when the
  /// server process cannot start.
  Future<void> activateConnection(AcpConnection connection) async {
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
    final previous = state.environment;
    state = AcpRuntimeState(
      environment: local,
      status: AcpConnectionStatus.disconnected,
    );
    if (!identical(previous, local)) {
      await previous?.close();
    }
  }

  /// Replaces the active environment with [environment], used by tests that
  /// cannot spawn a real ACP server process.
  @visibleForTesting
  void overrideEnvironmentForTest(RuntimeService runtime) {
    final previous = state.environment;
    state = AcpRuntimeState(
      environment: RuntimeEnvironment(
        runtime: runtime,
        models: const [],
        skills: const _EmptySkillCatalog(),
      ),
      status: AcpConnectionStatus.connected,
    );
    if (!identical(previous, _local)) {
      unawaited(previous?.close());
    }
  }
}

/// An empty skill catalog used for test environments.
final class _EmptySkillCatalog implements SkillCatalog {
  const _EmptySkillCatalog();

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
/// of whether the agent is local or remote.
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
        skills: loadSkillCatalog(homeDirectory: home),
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

import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_storage/atlas_storage.dart';

import '../features/remote_connection/application/runtime_controller.dart';
import 'acp_bootstrap.dart';
import 'remote_bootstrap.dart';

export '../features/remote_connection/application/runtime_controller.dart';

/// Creates process adapters that reuse the startup [environment] snapshot.
RuntimeEnvironmentController createRuntimeEnvironmentController({
  RuntimeEnvironment? local,
  Map<String, String>? environment,
}) {
  final snapshot = environment == null
      ? null
      : Map<String, String>.unmodifiable(environment);
  return RuntimeEnvironmentController(
    local: local,
    connectAcp: (connection) =>
        bootstrapAcpClient(connection, environment: snapshot),
    connectRemote: bootstrapRemoteConnection,
  );
}

/// Loads `~/.atlas/config.yaml` and composes the Flutter process runtime.
///
/// The local runtime is exposed through an in-process ACP server and consumed
/// through an [AcpClient]. Mobile clients skip local composition.
/// [environment] supplies both configuration substitutions and shell exports.
Future<RuntimeBootstrap> bootstrapRuntime({
  Map<String, String>? environment,
}) async {
  final values = Map<String, String>.unmodifiable(
    environment ?? Platform.environment,
  );
  final home = values['HOME'] ?? values['USERPROFILE'];
  if (home == null || home.isEmpty) {
    return const RuntimeBootstrap.failed(
      'Cannot locate the home directory for Atlas configuration.',
    );
  }

  final configFile = File('$home/.atlas/config.yaml');
  DriftSessionStore? store;
  ComposedTools? tools;
  DioHttpStreamClient? http;
  AgentRuntime? runtime;
  AcpClient? client;
  Future<void>? serverDone;
  Future<void>? closing;
  Future<void> cleanup() async {
    try {
      await runtime?.shutdown();
    } finally {
      try {
        await client?.close();
        await serverDone;
      } finally {
        try {
          await tools?.close();
        } finally {
          http?.close();
          await store?.close();
        }
      }
    }
  }

  Future<void> close() => closing ??= cleanup();

  try {
    final config = loadConfig(configFile, environment: values);
    final logger = composeLogger(config);
    tools = await composeTools(config, environment: values, logger: logger);
    http = DioHttpStreamClient();
    store = DriftSessionStore.openFile(File(config.session.dbPath));
    runtime = composeRuntime(
      config,
      store: store,
      tools: tools.registry,
      httpClient: http,
      logger: logger,
      shellEnvironment: values,
    );
    final models = List<ModelDescriptor>.unmodifiable(composeModels(config));
    final server = AcpServer(runtime, models: models);
    final (serving, clientTransport) = server.serveMemory();
    serverDone = serving;
    client = AcpClient(
      clientTransport,
      catalog: models,
      defaultModel: runtime.defaultModel,
    );
    await client.connect();
    return RuntimeBootstrap.ready(
      RuntimeEnvironment(
        runtime: client,
        models: client.catalog.isEmpty ? models : client.catalog,
        onClose: close,
      ),
    );
  } on ConfigLoadException catch (error) {
    await close();
    return RuntimeBootstrap.failed('Cannot load ${configFile.path}: $error');
  } on Object catch (error) {
    await close();
    final detail = error is SafeMessageException
        ? error.safeMessage
        : error.runtimeType.toString();
    return RuntimeBootstrap.failed('Cannot start Atlas: $detail');
  }
}

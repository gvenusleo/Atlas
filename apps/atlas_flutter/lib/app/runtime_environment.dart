import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
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
  try {
    final config = loadConfig(configFile, environment: values);
    store = DriftSessionStore.openFile(File(config.session.dbPath));
    final runtime = composeRuntime(
      config,
      store: store,
      shellEnvironment: values,
    );
    final models = List<ModelDescriptor>.unmodifiable(composeModels(config));
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

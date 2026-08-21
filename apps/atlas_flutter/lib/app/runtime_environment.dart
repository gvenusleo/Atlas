import 'dart:io';

import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

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
  final AgentRuntime runtime;

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
final runtimeEnvironmentProvider = Provider<RuntimeEnvironment?>((ref) => null);

/// Supplies a configuration or startup failure to the workspace.
final runtimeStartupErrorProvider = Provider<String?>((ref) => null);

/// Loads `~/.atlas/config.yaml` and composes the Flutter process runtime.
RuntimeBootstrap bootstrapRuntime({Map<String, String>? environment}) {
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
    return RuntimeBootstrap.ready(
      RuntimeEnvironment(
        runtime: runtime,
        models: List.unmodifiable(composeModels(config)),
        skills: loadSkillCatalog(homeDirectory: home),
        onClose: store.close,
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

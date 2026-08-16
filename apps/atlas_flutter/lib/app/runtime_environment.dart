import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Runtime services and catalogs injected into Flutter presentation code.
final class RuntimeEnvironment {
  /// Creates an environment around the shared agent runtime.
  const RuntimeEnvironment({
    required this.runtime,
    required this.models,
    required this.skills,
  });

  /// The single runtime used by every Flutter feature.
  final AgentRuntime runtime;

  /// Models configured for user selection.
  final List<ModelDescriptor> models;

  /// Skills available to slash-command completion.
  final SkillCatalog skills;
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
  try {
    final config = loadConfig(configFile);
    final tools = LocalToolRegistry([
      ReadTool(),
      WriteTool(),
      EditTool(),
      ShellTool(),
      PlanTool(),
    ]);
    final providers = <ProviderId, ModelProvider>{};
    final models = <ModelDescriptor>[];
    for (final configured in config.providers) {
      switch (configured) {
        case ConfiguredOpenAI(:final configuration):
          providers[configured.id] = OpenAICompatibleProvider([configuration]);
          models.addAll(configuration.models.map((model) => model.descriptor));
        case ConfiguredAnthropic(:final configuration):
          providers[configured.id] = AnthropicProvider([configuration]);
          models.addAll(configuration.models.map((model) => model.descriptor));
      }
    }

    final runtime = AgentRuntime(
      store: DriftSessionStore.openFile(File(config.session.dbPath)),
      provider: CompositeModelProvider(providers),
      tools: tools,
      ids: SecureIdGenerator(),
      defaultModel: config.defaultModel,
      sessionContextBuilder: buildSessionContext,
      maxSteps: config.agent.maxSteps,
      temperature: config.agent.temperature,
      compactionThreshold: config.agent.compaction.threshold,
      systemPromptBuilder: (context) => buildSystemPrompt(
        workingDirectory: context.workingDirectory,
        tools: tools.descriptors,
        instructions: context.instructions,
        skills: context.skills.summaries,
        now: DateTime.now(),
      ),
    );
    return RuntimeBootstrap.ready(
      RuntimeEnvironment(
        runtime: runtime,
        models: List.unmodifiable(models),
        skills: loadSkillCatalog(),
      ),
    );
  } on ConfigLoadException catch (error) {
    return RuntimeBootstrap.failed('Cannot load ${configFile.path}: $error');
  } on Object catch (error) {
    return RuntimeBootstrap.failed('Cannot start Atlas: $error');
  }
}

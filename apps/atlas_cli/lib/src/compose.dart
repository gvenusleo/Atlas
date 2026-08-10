import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';

/// Composes the configured runtime for one process.
///
/// Builds the model provider from [config], opens the session store at
/// [dbPath] or `config.session.dbPath`, and wires the tool registry and system
/// prompt builder into an [AgentRuntime].
AgentRuntime composeRuntime(
  AtlasConfig config, {
  SessionStore? store,
  ToolRegistry? tools,
  ModelProvider? provider,
  String? dbPath,
}) {
  final resolvedStore =
      store ??
      DriftSessionStore.openFile(File(dbPath ?? config.session.dbPath));
  final resolvedTools = tools ?? LocalToolRegistry(const []);

  // Index configured providers by their file-order identifier.
  final providers = <ProviderId, ModelProvider>{};
  for (final provider in config.providers) {
    switch (provider) {
      case ConfiguredOpenAI(:final configuration):
        providers[provider.id] = OpenAICompatibleProvider([configuration]);
      case ConfiguredAnthropic(:final configuration):
        providers[provider.id] = AnthropicProvider([configuration]);
    }
  }

  return AgentRuntime(
    store: resolvedStore,
    provider: provider ?? CompositeModelProvider(providers),
    tools: resolvedTools,
    ids: SecureIdGenerator(),
    defaultModel: config.defaultModel,
    maxSteps: config.agent.maxSteps,
    temperature: config.agent.temperature,
    systemPromptBuilder: (sessionId, request) => buildSystemPrompt(
      workingDirectory: request.workingDirectory ?? Directory.current.path,
      tools: resolvedTools.descriptors,
      instructions: loadInstructionFiles(
        workingDirectory: request.workingDirectory,
      ),
      now: DateTime.now(),
    ),
  );
}

/// Collects the configured model descriptors in file order.
///
/// Used by the TUI to offer model switching through `/model`.
List<ModelDescriptor> composeModels(AtlasConfig config) => [
  for (final provider in config.providers)
    switch (provider) {
      ConfiguredOpenAI(:final configuration) => configuration.models.map(
        (model) => model.descriptor,
      ),
      ConfiguredAnthropic(:final configuration) => configuration.models.map(
        (model) => model.descriptor,
      ),
    },
].expand((descriptors) => descriptors).toList();

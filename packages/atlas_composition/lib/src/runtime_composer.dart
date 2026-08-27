import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';

import 'logging.dart';

/// Composes the configured runtime for one process.
AgentRuntime composeRuntime(
  AtlasConfig config, {
  SessionStore? store,
  ToolRegistry? tools,
  ModelProvider? provider,
  SessionContext Function(String workingDirectory)? sessionContextBuilder,
  String? dbPath,
  AtlasLogger? logger,
}) {
  final resolvedTools =
      tools ??
      LocalToolRegistry([
        ReadTool(),
        WriteTool(),
        EditTool(),
        ShellTool(),
        PlanTool(),
      ]);
  final providers = <ProviderId, ModelProvider>{};
  for (final configured in config.providers) {
    switch (configured) {
      case ConfiguredOpenAI(:final configuration):
        providers[configured.id] = OpenAICompatibleProvider([configuration]);
      case ConfiguredAnthropic(:final configuration):
        providers[configured.id] = AnthropicProvider([configuration]);
    }
  }

  return AgentRuntime(
    store:
        store ??
        DriftSessionStore.openFile(File(dbPath ?? config.session.dbPath)),
    provider: provider ?? CompositeModelProvider(providers),
    tools: resolvedTools,
    ids: SecureIdGenerator(),
    defaultModel: config.defaultModel,
    sessionContextBuilder: sessionContextBuilder ?? buildSessionContext,
    maxSteps: config.agent.maxSteps,
    maxOutputTokens: config.agent.maxOutputTokens,
    logger:
        logger ??
        (config.logging.directory == null
            ? const NoopLogger()
            : FileLogSink(
                Directory(config.logging.directory!),
                minimumLevel: LogLevel.values.firstWhere(
                  (level) => level.name == config.logging.level,
                ),
                retainDays: config.logging.retainDays,
              )),
    temperature: config.agent.temperature,
    compactionThreshold: config.agent.compaction.threshold,
    systemPromptBuilder: (context) => buildSystemPrompt(
      workingDirectory: context.workingDirectory,
      tools: resolvedTools.descriptors,
      instructions: context.instructions,
      skills: context.skills.summaries,
      now: DateTime.now(),
    ),
  );
}

/// Returns configured model descriptors in provider and file order.
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

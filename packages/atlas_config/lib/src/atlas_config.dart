import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';

/// The application configuration loaded from `~/.atlas/config.yaml`.
final class const AtlasConfig({
  /// The model used when a turn does not provide an override.
  required final ModelRef defaultModel,

  /// Configured model providers in file order.
  required final List<ConfiguredProvider> providers,

  /// Agent loop parameters.
  required final AgentConfig agent,

  /// Local session storage settings.
  required final SessionConfig session,

  /// Structured file logging settings.
  final LoggingConfig logging = const LoggingConfig(),
}) {
  /// Creates application configuration.
  this;
}

/// Local structured logging settings.
final class const LoggingConfig({
  /// Minimum level written by file sinks (`debug`, `info`, `warn`, or `error`).
  final String level = 'info',

  /// Optional log directory. A null value leaves logging disabled by default.
  final String? directory,

  /// Number of daily log files to retain.
  final int retainDays = 7,
}) {
  /// Creates logging settings.
  this;
}

/// A configured model provider in its original file order.
sealed class const ConfiguredProvider({
  /// The provider identifier used by configured model references.
  required final ProviderId id,
}) {
  /// Creates a configured provider.
  this;
}

/// An OpenAI-compatible provider configuration.
final class const ConfiguredOpenAI({
  required super.id,

  /// The ready-to-use provider configuration object.
  required final OpenAIProviderConfiguration configuration,
}) extends ConfiguredProvider {
  /// Creates an OpenAI-compatible provider configuration.
  this;
}

/// An Anthropic provider configuration.
final class const ConfiguredAnthropic({
  required super.id,

  /// The ready-to-use provider configuration object.
  required final AnthropicProviderConfiguration configuration,
}) extends ConfiguredProvider {
  /// Creates an Anthropic provider configuration.
  this;
}

/// Agent loop parameters.
final class const AgentConfig({
  /// Maximum model/tool steps for one turn.
  final int maxSteps = 20,

  /// Maximum model output tokens for one step; zero uses provider defaults.
  final int maxOutputTokens = 0,

  /// Optional model sampling temperature.
  final double? temperature,

  /// Automatic context compaction settings.
  final CompactionConfig compaction = const CompactionConfig(),
}) {
  /// Creates agent parameters.
  this;
}

/// Automatic context compaction settings.
final class const CompactionConfig({
  /// Legacy fraction retained for compatibility with existing config files.
  final double threshold = 0.8,

  /// Approximate number of newest tokens retained verbatim.
  final int keepRecentTokens = 20000,

  /// Tokens reserved for the next model response.
  final int reserveTokens = 16384,
}) {
  /// Creates compaction settings.
  this;
}

/// Local session storage settings.
final class const SessionConfig(
  /// The expanded SQLite database path.
  final String dbPath,
) {
  /// Creates session settings.
  this;
}

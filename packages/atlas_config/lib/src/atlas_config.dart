import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';

/// The application configuration loaded from `~/.atlas/config.yaml`.
final class AtlasConfig {
  /// Creates application configuration.
  const AtlasConfig({
    required this.defaultModel,
    required this.providers,
    required this.agent,
    required this.session,
    this.logging = const LoggingConfig(),
  });

  /// The model used when a turn does not provide an override.
  final ModelRef defaultModel;

  /// Configured model providers in file order.
  final List<ConfiguredProvider> providers;

  /// Agent loop parameters.
  final AgentConfig agent;

  /// Local session storage settings.
  final SessionConfig session;

  /// Structured file logging settings.
  final LoggingConfig logging;
}

/// Local structured logging settings.
final class LoggingConfig {
  /// Creates logging settings.
  const LoggingConfig({
    this.level = 'info',
    this.directory,
    this.retainDays = 7,
  });

  /// Minimum level written by file sinks (`debug`, `info`, `warn`, or `error`).
  final String level;

  /// Optional log directory. A null value leaves logging disabled by default.
  final String? directory;

  /// Number of daily log files to retain.
  final int retainDays;
}

/// A configured model provider in its original file order.
sealed class ConfiguredProvider {
  /// Creates a configured provider.
  const ConfiguredProvider({required this.id});

  /// The provider identifier used by configured model references.
  final ProviderId id;
}

/// An OpenAI-compatible provider configuration.
final class ConfiguredOpenAI extends ConfiguredProvider {
  /// Creates an OpenAI-compatible provider configuration.
  const ConfiguredOpenAI({required super.id, required this.configuration});

  /// The ready-to-use provider configuration object.
  final OpenAIProviderConfiguration configuration;
}

/// An Anthropic provider configuration.
final class ConfiguredAnthropic extends ConfiguredProvider {
  /// Creates an Anthropic provider configuration.
  const ConfiguredAnthropic({required super.id, required this.configuration});

  /// The ready-to-use provider configuration object.
  final AnthropicProviderConfiguration configuration;
}

/// Agent loop parameters.
final class AgentConfig {
  /// Creates agent parameters.
  const AgentConfig({
    this.maxSteps = 20,
    this.maxOutputTokens = 0,
    this.temperature,
    this.compaction = const CompactionConfig(),
  });

  /// Maximum model/tool steps for one turn.
  final int maxSteps;

  /// Maximum model output tokens for one step; zero uses provider defaults.
  final int maxOutputTokens;

  /// Optional model sampling temperature.
  final double? temperature;

  /// Automatic context compaction settings.
  final CompactionConfig compaction;
}

/// Automatic context compaction settings.
final class CompactionConfig {
  /// Creates compaction settings.
  const CompactionConfig({
    this.threshold = 0.8,
    this.keepRecentTokens = 20000,
    this.reserveTokens = 16384,
  });

  /// Legacy fraction retained for compatibility with existing config files.
  final double threshold;

  /// Approximate number of newest tokens retained verbatim.
  final int keepRecentTokens;

  /// Tokens reserved for the next model response.
  final int reserveTokens;
}

/// Local session storage settings.
final class SessionConfig {
  /// Creates session settings.
  const SessionConfig(this.dbPath);

  /// The expanded SQLite database path.
  final String dbPath;
}

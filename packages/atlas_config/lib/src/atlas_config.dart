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
  });

  /// The model used when a turn does not provide an override.
  final ModelRef defaultModel;

  /// Configured model providers in file order.
  final List<ConfiguredProvider> providers;

  /// Agent loop parameters.
  final AgentConfig agent;

  /// Local session storage settings.
  final SessionConfig session;
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
  const AgentConfig({this.maxSteps = 100, this.temperature});

  /// Maximum model/tool steps for one turn.
  final int maxSteps;

  /// Optional model sampling temperature.
  final double? temperature;
}

/// Local session storage settings.
final class SessionConfig {
  /// Creates session settings.
  const SessionConfig(this.dbPath);

  /// The expanded SQLite database path.
  final String dbPath;
}

import 'dart:io';

import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:yaml/yaml.dart';

import 'atlas_config.dart';

/// Raised when the configuration file cannot be parsed or validated.
final class ConfigLoadException implements Exception {
  /// Creates a configuration failure.
  const ConfigLoadException(this.message);

  /// A user-visible message with a field path when available.
  final String message;

  @override
  String toString() => message;
}

/// Parses [yamlText] into application configuration.
///
/// [environment] supplies values for `${VAR}` references in `api_key`
/// fields; [homeDirectory] expands a leading `~/` in `db_path`.
AtlasConfig parseConfig(
  String yamlText, {
  Map<String, String>? environment,
  String? homeDirectory,
}) {
  final env = environment ?? Platform.environment;
  final home = homeDirectory ?? env['HOME'];
  final root = _asMap(_loadDocument(yamlText), '');
  final providers = _providers(root, env);
  if (providers.isEmpty) {
    throw const ConfigLoadException('providers must not be empty');
  }
  return AtlasConfig(
    defaultModel: _defaultModel(root, providers),
    providers: providers,
    agent: _agent(root),
    session: _session(root, home),
    logging: _logging(root, home, env),
  );
}

LoggingConfig _logging(
  Map<String, Object?> root,
  String? home,
  Map<String, String> environment,
) {
  final map = _asMap(root['logging'] ?? const <String, Object?>{}, 'logging');
  final rawLevel =
      (_stringOrNull(map['level'], 'logging.level') ??
              environment['ATLAS_LOG_LEVEL'] ??
              'info')
          .toLowerCase();
  if (!{'debug', 'info', 'warn', 'error'}.contains(rawLevel)) {
    throw ConfigLoadException(
      'logging.level must be debug, info, warn, or error',
    );
  }
  final rawDirectory = _stringOrNull(map['directory'], 'logging.directory');
  final retainDays = _positiveInt(map['retain_days'], 'logging.retain_days', 7);
  return LoggingConfig(
    level: rawLevel,
    directory: rawDirectory == null ? null : _expandHome(rawDirectory, home),
    retainDays: retainDays,
  );
}

/// Reads and parses the configuration file at [file].
AtlasConfig loadConfig(File file) {
  final String yamlText;
  try {
    yamlText = file.readAsStringSync();
  } on FileSystemException catch (error) {
    throw ConfigLoadException('cannot read ${file.path}: ${error.message}');
  }
  return parseConfig(yamlText);
}

Object? _loadDocument(String yamlText) {
  try {
    return loadYaml(yamlText);
  } on YamlException catch (error) {
    throw ConfigLoadException('invalid YAML: ${error.message}');
  }
}

List<ConfiguredProvider> _providers(
  Map<String, Object?> root,
  Map<String, String> env,
) {
  final rawProviders = _asList(root['providers'], 'providers');
  final seen = <String>{};
  final result = <ConfiguredProvider>[];
  for (final (index, raw) in rawProviders.indexed) {
    final path = 'providers[$index]';
    final map = _asMap(raw, path);
    final name = _string(map['name'], '$path.name');
    if (!seen.add(name)) {
      throw ConfigLoadException('$path.name duplicates provider "$name"');
    }
    switch (_string(map['type'], '$path.type')) {
      case 'chat_completions':
        result.add(
          ConfiguredOpenAI(
            id: ProviderId(name),
            configuration: _openAIConfiguration(
              map,
              name,
              path,
              env,
              OpenAIProtocol.chatCompletions,
            ),
          ),
        );
      case 'responses':
        result.add(
          ConfiguredOpenAI(
            id: ProviderId(name),
            configuration: _openAIConfiguration(
              map,
              name,
              path,
              env,
              OpenAIProtocol.responses,
            ),
          ),
        );
      case 'anthropic':
        result.add(
          ConfiguredAnthropic(
            id: ProviderId(name),
            configuration: _anthropicConfiguration(map, name, path, env),
          ),
        );
      default:
        throw ConfigLoadException(
          '$path.type must be "chat_completions", "responses", or '
          '"anthropic"',
        );
    }
  }
  return result;
}

OpenAIProviderConfiguration _openAIConfiguration(
  Map<String, Object?> map,
  String name,
  String path,
  Map<String, String> env,
  OpenAIProtocol protocol,
) {
  return OpenAIProviderConfiguration(
    id: ProviderId(name),
    protocol: protocol,
    baseUrl: _baseUrl(map['base_url'], '$path.base_url'),
    apiKey: _expandEnv(
      _string(map['api_key'], '$path.api_key'),
      '$path.api_key',
      env,
    ),
    userAgent: _stringOrNull(map['user_agent'], '$path.user_agent'),
    models: _openAIModels(map, name, path),
  );
}

AnthropicProviderConfiguration _anthropicConfiguration(
  Map<String, Object?> map,
  String name,
  String path,
  Map<String, String> env,
) {
  return AnthropicProviderConfiguration(
    id: ProviderId(name),
    baseUrl: _baseUrl(map['base_url'], '$path.base_url'),
    apiKey: _expandEnv(
      _string(map['api_key'], '$path.api_key'),
      '$path.api_key',
      env,
    ),
    apiVersion:
        _stringOrNull(map['api_version'], '$path.api_version') ?? '2023-06-01',
    userAgent: _stringOrNull(map['user_agent'], '$path.user_agent'),
    models: _anthropicModels(map, name, path),
  );
}

List<OpenAIModelConfiguration> _openAIModels(
  Map<String, Object?> map,
  String name,
  String path,
) {
  final rawModels = _asList(map['models'], '$path.models');
  if (rawModels.isEmpty) {
    throw ConfigLoadException('$path.models must not be empty');
  }
  final seen = <String>{};
  return [
    for (final (index, raw) in rawModels.indexed)
      _openAIModel(
        _asMap(raw, '$path.models[$index]'),
        name,
        '$path.models[$index]',
        seen,
      ),
  ];
}

OpenAIModelConfiguration _openAIModel(
  Map<String, Object?> map,
  String providerName,
  String path,
  Set<String> seen,
) {
  final id = _modelId(map, path, seen);
  return OpenAIModelConfiguration(
    descriptor: _descriptor(map, providerName, path, id),
    promptCacheEnabled: _boolDefault(
      map['prompt_cache'],
      '$path.prompt_cache',
      false,
    ),
  );
}

List<AnthropicModelConfiguration> _anthropicModels(
  Map<String, Object?> map,
  String name,
  String path,
) {
  final rawModels = _asList(map['models'], '$path.models');
  if (rawModels.isEmpty) {
    throw ConfigLoadException('$path.models must not be empty');
  }
  final seen = <String>{};
  return [
    for (final (index, raw) in rawModels.indexed)
      _anthropicModel(
        _asMap(raw, '$path.models[$index]'),
        name,
        '$path.models[$index]',
        seen,
      ),
  ];
}

AnthropicModelConfiguration _anthropicModel(
  Map<String, Object?> map,
  String providerName,
  String path,
  Set<String> seen,
) {
  final id = _modelId(map, path, seen);
  final descriptor = _descriptor(map, providerName, path, id);
  final thinkingBudget = _nonNegativeInt(
    map['thinking_budget_tokens'],
    '$path.thinking_budget_tokens',
    0,
  );
  if (descriptor.maxOutputTokens > 0 &&
      thinkingBudget >= descriptor.maxOutputTokens) {
    throw ConfigLoadException(
      '$path.thinking_budget_tokens must be less than $path.max_tokens',
    );
  }
  return AnthropicModelConfiguration(
    descriptor: descriptor,
    thinkingBudgetTokens: thinkingBudget,
  );
}

String _modelId(Map<String, Object?> map, String path, Set<String> seen) {
  final id = _string(map['value'], '$path.value');
  if (!seen.add(id)) {
    throw ConfigLoadException('$path.id duplicates model "$id"');
  }
  return id;
}

ModelDescriptor _descriptor(
  Map<String, Object?> map,
  String providerName,
  String path,
  String id,
) => ModelDescriptor(
  ref: ModelRef(providerId: ProviderId(providerName), modelId: ModelId(id)),
  name: _stringOrNull(map['name'], '$path.name') ?? '',
  description: _stringOrNull(map['description'], '$path.description') ?? '',
  contextWindow: _nonNegativeInt(
    map['context_window'],
    '$path.context_window',
    0,
  ),
  maxOutputTokens: _nonNegativeInt(map['max_tokens'], '$path.max_tokens', 0),
  inputCapabilities: _inputCapabilities(
    map['input_capabilities'],
    '$path.input_capabilities',
  ),
  reasoningEfforts: _reasoningEfforts(
    map['reasoning_efforts'],
    '$path.reasoning_efforts',
  ),
);

ModelRef _defaultModel(
  Map<String, Object?> root,
  List<ConfiguredProvider> providers,
) {
  final raw = _string(root['default_model'], 'default_model');
  final separator = raw.indexOf('/');
  if (separator <= 0 || separator == raw.length - 1) {
    throw const ConfigLoadException(
      'default_model must be "<provider>/<model>"',
    );
  }
  final providerName = raw.substring(0, separator);
  final modelId = raw.substring(separator + 1);
  final provider = providers.firstWhere(
    (candidate) => candidate.id.value == providerName,
    orElse: () => throw ConfigLoadException(
      'default_model references unknown provider "$providerName"',
    ),
  );
  final known = switch (provider) {
    ConfiguredOpenAI(:final configuration) => configuration.models.any(
      (model) => model.descriptor.ref.modelId.value == modelId,
    ),
    ConfiguredAnthropic(:final configuration) => configuration.models.any(
      (model) => model.descriptor.ref.modelId.value == modelId,
    ),
  };
  if (!known) {
    throw ConfigLoadException(
      'default_model references unknown model "$modelId"',
    );
  }
  return ModelRef(providerId: provider.id, modelId: ModelId(modelId));
}

AgentConfig _agent(Map<String, Object?> root) {
  final map = _asMap(root['agent'] ?? const <String, Object?>{}, 'agent');
  final compaction = _asMap(
    map['compaction'] ?? const <String, Object?>{},
    'agent.compaction',
  );
  return AgentConfig(
    maxSteps: _positiveInt(map['max_steps'], 'agent.max_steps', 20),
    maxOutputTokens: _nonNegativeInt(
      map['max_output_tokens'],
      'agent.max_output_tokens',
      0,
    ),
    temperature: _doubleOrNull(map['temperature'], 'agent.temperature'),
    compaction: CompactionConfig(
      threshold: _fraction(
        compaction['threshold'],
        'agent.compaction.threshold',
        0.8,
      ),
      keepRecentTokens: _positiveInt(
        compaction['keep_recent_tokens'],
        'agent.compaction.keep_recent_tokens',
        20000,
      ),
      reserveTokens: _positiveInt(
        compaction['reserve_tokens'],
        'agent.compaction.reserve_tokens',
        16384,
      ),
    ),
  );
}

SessionConfig _session(Map<String, Object?> root, String? home) {
  final map = _asMap(root['session'] ?? const <String, Object?>{}, 'session');
  final raw =
      _stringOrNull(map['db_path'], 'session.db_path') ?? '~/.atlas/atlas.db';
  return SessionConfig(_expandHome(raw, home));
}

String _expandHome(String path, String? home) {
  if (home == null || home.isEmpty || !path.startsWith('~/')) {
    return path;
  }
  return '$home${path.substring(1)}';
}

String _expandEnv(String value, String path, Map<String, String> env) {
  final pattern = RegExp(r'\$\{([A-Za-z_][A-Za-z0-9_]*)\}');
  return value.replaceAllMapped(pattern, (match) {
    final name = match.group(1)!;
    final replacement = env[name];
    if (replacement == null) {
      throw ConfigLoadException(
        '$path references undefined environment variable \$$name',
      );
    }
    return replacement;
  });
}

Uri _baseUrl(Object? value, String path) {
  final text = _string(value, path);
  final uri = Uri.tryParse(text);
  if (uri == null ||
      (uri.scheme != 'http' && uri.scheme != 'https') ||
      uri.host.isEmpty ||
      uri.hasQuery ||
      uri.hasFragment) {
    throw ConfigLoadException(
      '$path must be an HTTP URL without a query or fragment',
    );
  }
  return uri;
}

Set<ModelInputCapability> _inputCapabilities(Object? value, String path) {
  if (value == null) {
    return const {ModelInputCapability.text};
  }
  final list = _asList(value, path);
  final result = <ModelInputCapability>{};
  for (final (index, raw) in list.indexed) {
    switch (raw) {
      case 'text':
        result.add(ModelInputCapability.text);
      case 'image':
        result.add(ModelInputCapability.image);
      default:
        throw ConfigLoadException('$path[$index] must be "text" or "image"');
    }
  }
  return result;
}

List<ReasoningEffortOption> _reasoningEfforts(Object? value, String path) {
  if (value == null) {
    return const [];
  }
  final list = _asList(value, path);
  final result = <ReasoningEffortOption>[];
  for (final (index, raw) in list.indexed) {
    final map = _asMap(raw, '$path[$index]');
    result.add(
      ReasoningEffortOption(
        value: _string(map['value'], '$path[$index].value'),
        name: _stringOrNull(map['name'], '$path[$index].name') ?? '',
        description:
            _stringOrNull(map['description'], '$path[$index].description') ??
            '',
      ),
    );
  }
  return result;
}

Map<String, Object?> _asMap(Object? value, String path) {
  if (value is Map) {
    return value.map((key, nested) => MapEntry(key.toString(), nested));
  }
  throw ConfigLoadException('$path must be a map');
}

List<Object?> _asList(Object? value, String path) {
  if (value is List) {
    return value;
  }
  throw ConfigLoadException('$path must be a list');
}

String _string(Object? value, String path) {
  if (value is String && value.isNotEmpty) {
    return value;
  }
  throw ConfigLoadException('$path must be a non-empty string');
}

String? _stringOrNull(Object? value, String path) {
  if (value == null) {
    return null;
  }
  if (value is String) {
    return value;
  }
  throw ConfigLoadException('$path must be a string');
}

bool _boolDefault(Object? value, String path, bool fallback) {
  if (value == null) {
    return fallback;
  }
  if (value is bool) {
    return value;
  }
  throw ConfigLoadException('$path must be a boolean');
}

int _intDefault(Object? value, String path, int fallback) {
  if (value == null) {
    return fallback;
  }
  if (value is int) {
    return value;
  }
  throw ConfigLoadException('$path must be an integer');
}

int _nonNegativeInt(Object? value, String path, int fallback) {
  final result = _intDefault(value, path, fallback);
  if (result < 0) {
    throw ConfigLoadException('$path must not be negative');
  }
  return result;
}

int _positiveInt(Object? value, String path, int fallback) {
  final result = _intDefault(value, path, fallback);
  if (result <= 0) {
    throw ConfigLoadException('$path must be greater than zero');
  }
  return result;
}

double? _doubleOrNull(Object? value, String path) {
  if (value == null) {
    return null;
  }
  if (value is num) {
    return value.toDouble();
  }
  throw ConfigLoadException('$path must be a number');
}

double _fraction(Object? value, String path, double fallback) {
  final result = _doubleOrNull(value, path) ?? fallback;
  if (result <= 0 || result > 1) {
    throw ConfigLoadException('$path must be greater than 0 and at most 1');
  }
  return result;
}

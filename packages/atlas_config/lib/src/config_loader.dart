import 'dart:io';

import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:path/path.dart' as p;

import 'atlas_config.dart';
import 'mcp_config.dart';

part 'models_loader.dart';
part 'mcp_loader.dart';

/// A configuration failure with a file or field path and no secret values.
final class const ConfigLoadException(
  /// The safe file or field diagnostic.
  final String message,
) implements Exception {
  /// Creates a configuration diagnostic.
  this;
  @override
  String toString() => message;
}

/// Loads Pi-shaped settings, model overrides, credentials and MCP configuration.
///
/// Missing documents use defaults. Legacy YAML is rejected, never migrated.
AtlasConfig loadConfig(
  Directory directory, {
  Map<String, String>? environment,
}) {
  final env = environment ?? Platform.environment;
  if (File(p.join(directory.path, 'config.yaml')).existsSync()) {
    throw const ConfigLoadException(
      'config.yaml is no longer supported; replace it with settings.json, models.json and mcp.json',
    );
  }
  String read(String name) {
    final file = File(p.join(directory.path, name));
    if (!file.existsSync()) return '{}';
    try {
      return file.readAsStringSync();
    } on FileSystemException {
      throw ConfigLoadException('Cannot read ${file.path}');
    }
  }

  return parseConfig(
    read('settings.json'),
    modelsText: read('models.json'),
    mcpText: read('mcp.json'),
    environment: env,
    homeDirectory: env['HOME'] ?? env['USERPROFILE'],
    atlasDirectory: directory,
    authStore: AuthStore(File(p.join(directory.path, 'auth.json'))),
    catalog: ModelCatalog.load(directory),
  );
}

/// Parses independently owned JSON documents without reading credentials.
AtlasConfig parseConfig(
  String settingsText, {
  String modelsText = '{}',
  String mcpText = '{}',
  Map<String, String>? environment,
  String? homeDirectory,
  Directory? atlasDirectory,
  AuthStore? authStore,
  ModelCatalog? catalog,
}) {
  final env = environment ?? Platform.environment;
  final home = homeDirectory ?? env['HOME'] ?? env['USERPROFILE'];
  final settings = _document(settingsText, 'settings.json');
  _keys(settings, {
    'defaultProvider',
    'defaultModel',
    'defaultThinkingLevel',
    'agent',
    'compaction',
    'session',
    'logging',
  }, 'settings.json');
  final thinking = _optionalString(
    settings['defaultThinkingLevel'],
    'settings.defaultThinkingLevel',
  );
  if (thinking != null && !_thinkingLevels.contains(thinking)) {
    throw const ConfigLoadException(
      'settings.defaultThinkingLevel is unsupported',
    );
  }
  final providers = _providers(
    _document(modelsText, 'models.json'),
    catalog ?? ModelCatalog.bundled(),
    env,
    authStore,
    thinking,
  );
  final providerId =
      _optionalString(
        settings['defaultProvider'],
        'settings.defaultProvider',
      ) ??
      'openai';
  final candidates = <ModelDescriptor>[
    for (final provider in providers)
      ...switch (provider) {
        ConfiguredOpenAI(:final configuration) => configuration.models.map(
          (m) => m.descriptor,
        ),
        ConfiguredAnthropic(:final configuration) => configuration.models.map(
          (m) => m.descriptor,
        ),
      },
  ];
  final modelId =
      _optionalString(settings['defaultModel'], 'settings.defaultModel') ??
      candidates
          .where((m) => m.ref.providerId.value == providerId)
          .firstOrNull
          ?.ref
          .modelId
          .value;
  if (modelId == null ||
      !candidates.any(
        (m) =>
            m.ref.providerId.value == providerId &&
            m.ref.modelId.value == modelId,
      )) {
    throw const ConfigLoadException(
      'settings.defaultProvider/defaultModel references an unknown model',
    );
  }
  final agent = _map(settings['agent'] ?? {}, 'settings.agent');
  _keys(agent, {
    'maxSteps',
    'maxOutputTokens',
    'temperature',
  }, 'settings.agent');
  final compaction = _map(settings['compaction'] ?? {}, 'settings.compaction');
  _keys(compaction, {
    'threshold',
    'keepRecentTokens',
    'reserveTokens',
  }, 'settings.compaction');
  final session = _map(settings['session'] ?? {}, 'settings.session');
  _keys(session, {'dbPath'}, 'settings.session');
  final logging = _map(settings['logging'] ?? {}, 'settings.logging');
  _keys(logging, {'level', 'directory', 'retainDays'}, 'settings.logging');
  final level =
      (_optionalString(logging['level'], 'settings.logging.level') ??
              env['ATLAS_LOG_LEVEL'] ??
              'info')
          .toLowerCase();
  if (!{'debug', 'info', 'warn', 'error'}.contains(level)) {
    throw const ConfigLoadException('settings.logging.level is invalid');
  }
  final threshold =
      _number(compaction['threshold'], 'settings.compaction.threshold') ?? 0.8;
  if (threshold <= 0 || threshold > 1) {
    throw const ConfigLoadException(
      'settings.compaction.threshold must be in (0, 1]',
    );
  }
  final logDirectory = _optionalString(
    logging['directory'],
    'settings.logging.directory',
  );
  return AtlasConfig(
    defaultModel: ModelRef(
      providerId: ProviderId(providerId),
      modelId: ModelId(modelId),
    ),
    providers: List.unmodifiable(providers),
    mcpServers: _mcpServers(_document(mcpText, 'mcp.json'), env, home),
    agent: AgentConfig(
      maxSteps: _integer(agent['maxSteps'], 'settings.agent.maxSteps', 20),
      maxOutputTokens: _integer(
        agent['maxOutputTokens'],
        'settings.agent.maxOutputTokens',
        0,
        minimum: 0,
      ),
      temperature: _number(agent['temperature'], 'settings.agent.temperature'),
      compaction: CompactionConfig(
        threshold: threshold,
        keepRecentTokens: _integer(
          compaction['keepRecentTokens'],
          'settings.compaction.keepRecentTokens',
          20000,
        ),
        reserveTokens: _integer(
          compaction['reserveTokens'],
          'settings.compaction.reserveTokens',
          16384,
        ),
      ),
    ),
    session: SessionConfig(
      _expandHome(
        _optionalString(session['dbPath'], 'settings.session.dbPath') ??
            p.join(
              atlasDirectory?.path ?? p.join(home ?? '.', '.atlas'),
              'atlas.db',
            ),
        home,
      ),
    ),
    logging: LoggingConfig(
      level: level,
      directory: logDirectory == null ? null : _expandHome(logDirectory, home),
      retainDays: _integer(
        logging['retainDays'],
        'settings.logging.retainDays',
        7,
      ),
    ),
  );
}

Map<String, Object?> _document(String text, String name) {
  try {
    return decodeJsonDocument(text);
  } on FormatException catch (error) {
    throw ConfigLoadException(
      '$name: invalid JSON at offset ${error.offset ?? 0}',
    );
  }
}

Map<String, Object?> _map(Object? value, String path) {
  if (value is Map && value.keys.every((key) => key is String)) {
    return Map<String, Object?>.from(value);
  }
  throw ConfigLoadException('$path must be an object');
}

List<Object?> _list(Object? value, String path) {
  if (value is List) return value;
  throw ConfigLoadException('$path must be an array');
}

void _keys(Map<String, Object?> map, Set<String> allowed, String path) {
  for (final key in map.keys) {
    if (!allowed.contains(key)) {
      throw ConfigLoadException('$path.$key is unsupported');
    }
  }
}

String _string(Object? value, String path) {
  if (value is String && value.isNotEmpty) return value;
  throw ConfigLoadException('$path must be a non-empty string');
}

String? _optionalString(Object? value, String path) =>
    value == null ? null : _string(value, path);

int _integer(Object? value, String path, int fallback, {int minimum = 1}) {
  if (value == null) return fallback;
  if (value is int && value >= minimum) return value;
  throw ConfigLoadException('$path must be an integer >= $minimum');
}

double? _number(Object? value, String path) {
  if (value == null) return null;
  if (value is num && value.isFinite) return value.toDouble();
  throw ConfigLoadException('$path must be a finite number');
}

bool _boolean(Object? value, String path, bool fallback) {
  if (value == null) return fallback;
  if (value is bool) return value;
  throw ConfigLoadException('$path must be a boolean');
}

String _expandHome(String value, String? home) =>
    home != null && value.startsWith('~/')
    ? p.join(home, value.substring(2))
    : value;

Uri _url(Object? value, String path, {bool query = false}) {
  final uri = Uri.tryParse(_string(value, path));
  if (uri == null ||
      !{'http', 'https'}.contains(uri.scheme) ||
      uri.host.isEmpty ||
      uri.userInfo.isNotEmpty ||
      uri.hasFragment ||
      (!query && uri.hasQuery)) {
    throw ConfigLoadException(
      '$path must be an HTTP(S) URL without userinfo, fragment${query ? '' : ', or query'}',
    );
  }
  return uri;
}

Map<String, String> _strings(Object? value, String path) {
  if (value == null) return {};
  return _map(value, path).map((key, value) {
    if (value is! String) {
      throw ConfigLoadException('$path.$key must be a string');
    }
    return MapEntry(key, value);
  });
}

part of 'config_loader.dart';

const _thinkingLevels = {
  'off',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
};
const _modelFields = {
  'id',
  'name',
  'api',
  'baseUrl',
  'reasoning',
  'thinkingLevelMap',
  'input',
  'cost',
  'contextWindow',
  'maxTokens',
  'samplingParams',
  'samplingParamsByThinkingLevel',
  'headers',
  'compat',
};
const _overrideFields = {
  'name',
  'reasoning',
  'thinkingLevelMap',
  'input',
  'cost',
  'contextWindow',
  'maxTokens',
  'samplingParams',
  'samplingParamsByThinkingLevel',
  'headers',
  'compat',
};

List<ConfiguredProvider> _providers(
  Map<String, Object?> document,
  ModelCatalog catalog,
  Map<String, String> env,
  AuthStore? authStore,
  String? defaultThinking,
) {
  _keys(document, {'providers'}, 'models');
  final configured = _map(document['providers'] ?? {}, 'models.providers');
  final ids = {...catalog.providers.keys, ...configured.keys};
  final result = <ConfiguredProvider>[];
  for (final id in ids) {
    final path = 'models.providers.$id';
    if (id.isEmpty || id.contains('/') || id.trim() != id) {
      throw ConfigLoadException('$path is not a valid provider ID');
    }
    final config = _map(configured[id] ?? {}, path);
    _keys(config, {
      'name',
      'baseUrl',
      'apiKey',
      'api',
      'headers',
      'compat',
      'authHeader',
      'models',
      'modelOverrides',
    }, path);
    _optionalString(config['name'], '$path.name');
    final preset = providerPresets[id];
    final providerApi = _optionalString(config['api'], '$path.api');
    if (providerApi != null) _protocol(providerApi, '$path.api');
    final providerUrl = config['baseUrl'] == null
        ? null
        : _url(config['baseUrl'], '$path.baseUrl').toString();
    final key = _optionalString(config['apiKey'], '$path.apiKey');
    final authHeader = _boolean(
      config['authHeader'],
      '$path.authHeader',
      false,
    );
    final headers = _headers(config['headers'], '$path.headers');
    final providerCompat = _map(config['compat'] ?? {}, '$path.compat');
    final models = <String, Map<String, Object?>>{
      for (final model in catalog.providers[id] ?? <Map<String, Object?>>[])
        model['id'] as String: {
          ...model,
          'baseUrl': providerUrl ?? model['baseUrl'],
          'compat': {..._map(model['compat'] ?? {}, path), ...providerCompat},
          'headers': _headers(
            model['headers'],
            '$path.models.${model['id']}.headers',
          ),
        },
    };
    final seen = <String>{};
    for (final (index, raw) in _list(
      config['models'] ?? [],
      '$path.models',
    ).indexed) {
      final mp = '$path.models[$index]';
      final model = _map(raw, mp);
      _keys(model, _modelFields, mp);
      final modelId = _string(model['id'], '$mp.id');
      if (!seen.add(modelId)) throw ConfigLoadException('$mp.id is duplicated');
      final requestedApi = model['api'] ?? providerApi;
      final previous =
          models[modelId] ??
          (requestedApi == null
              ? null
              : models.values
                    .where((m) => m['api'] == requestedApi)
                    .firstOrNull) ??
          models.values
              .where((m) => m['api'] == 'openai-completions')
              .firstOrNull ??
          models.values.firstOrNull;
      final api =
          model['api'] ?? providerApi ?? previous?['api'] ?? preset?.api;
      final baseUrl =
          model['baseUrl'] ??
          providerUrl ??
          previous?['baseUrl'] ??
          preset?.baseUrl;
      // Pi models entries are definitions; metadata defaults do not silently
      // inherit capabilities from a similarly named model on another provider.
      models[modelId] = {
        ...model,
        'headers': _headers(model['headers'], '$mp.headers'),
        'api': api,
        'baseUrl': baseUrl,
        'compat': {
          ...providerCompat,
          ..._map(model['compat'] ?? {}, '$mp.compat'),
        },
      };
    }
    final overrides = _map(
      config['modelOverrides'] ?? {},
      '$path.modelOverrides',
    );
    for (final entry in overrides.entries) {
      final op = '$path.modelOverrides.${entry.key}';
      final override = _map(entry.value, op);
      _keys(override, _overrideFields, op);
      final base = models[entry.key];
      if (base == null) continue; // Pi ignores overrides for unknown IDs.
      models[entry.key] = _mergeModel(base, {
        ...override,
        if (override.containsKey('headers'))
          'headers': _headers(override['headers'], '$op.headers'),
      });
    }
    if (models.isEmpty) {
      throw ConfigLoadException(
        '$path.models must define at least one model for a custom provider',
      );
    }
    for (final entry in models.entries) {
      final mp = '$path.models.${entry.key}';
      final model = entry.value;
      final api = _protocol(_string(model['api'], '$mp.api'), '$mp.api');
      final baseUrl = _url(model['baseUrl'], '$mp.baseUrl');
      final options = _options(model, mp, api, defaultThinking);
      final inputs = <ModelInputCapability>{};
      for (final input in _list(model['input'] ?? ['text'], '$mp.input')) {
        switch (input) {
          case 'text':
            inputs.add(ModelInputCapability.text);
          case 'image':
            inputs.add(ModelInputCapability.image);
          default:
            throw ConfigLoadException('$mp.input only supports text and image');
        }
      }
      final levels = options.reasoning
          ? (options.thinkingLevelMap.isNotEmpty
                ? options.thinkingLevelMap.keys.toList()
                : ['off', 'minimal', 'low', 'medium', 'high'])
          : <String>[];
      final preferred = options.defaultThinkingLevel;
      if (levels.remove(preferred)) levels.insert(0, preferred!);
      final descriptor = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId(id), modelId: ModelId(entry.key)),
        name: _optionalString(model['name'], '$mp.name') ?? entry.key,
        contextWindow: _integer(
          model['contextWindow'],
          '$mp.contextWindow',
          128000,
        ),
        maxOutputTokens: _integer(model['maxTokens'], '$mp.maxTokens', 16384),
        inputCapabilities: Set.unmodifiable(inputs),
        reasoningEfforts: List.unmodifiable([
          for (final level in levels) ReasoningEffortOption(value: level),
        ]),
      );
      final authentication = ProviderAuthentication(
        provider: id,
        resolver: ConfigValueResolver(env),
        store: authStore,
        apiKey: key,
        environmentKeys: preset?.env ?? const [],
        headers: Map.unmodifiable({
          ...headers,
          ..._headers(model['headers'], '$mp.headers'),
        }),
        authHeader: authHeader,
      );
      switch (api) {
        case 'anthropic-messages':
          result.add(
            ConfiguredAnthropic(
              id: ProviderId(id),
              configuration: AnthropicProviderConfiguration(
                id: ProviderId(id),
                baseUrl: baseUrl,
                authentication: authentication,
                models: [
                  AnthropicModelConfiguration(
                    descriptor: descriptor,
                    options: options,
                  ),
                ],
              ),
            ),
          );
        case 'openai-completions':
        case 'openai-responses':
          result.add(
            ConfiguredOpenAI(
              id: ProviderId(id),
              configuration: OpenAIProviderConfiguration(
                id: ProviderId(id),
                baseUrl: baseUrl,
                authentication: authentication,
                protocol: api == 'openai-responses'
                    ? OpenAIProtocol.responses
                    : OpenAIProtocol.chatCompletions,
                models: [
                  OpenAIModelConfiguration(
                    descriptor: descriptor,
                    options: options,
                  ),
                ],
              ),
            ),
          );
      }
    }
  }
  return result;
}

String _protocol(String api, String path) {
  if (!{
    'openai-completions',
    'openai-responses',
    'anthropic-messages',
  }.contains(api)) {
    throw ConfigLoadException('$path is not an implemented API');
  }
  return api;
}

Map<String, Object?> _mergeModel(
  Map<String, Object?> base,
  Map<String, Object?> override,
) => {
  ...base,
  for (final entry in override.entries)
    entry.key:
        base[entry.key] is Map<String, Object?> &&
            entry.value is Map<String, Object?>
        ? _mergeModel(
            base[entry.key] as Map<String, Object?>,
            entry.value as Map<String, Object?>,
          )
        : entry.value,
};

Map<String, String> _headers(Object? raw, String path) {
  final headers = _strings(raw, path);
  final result = <String, String>{};
  for (final entry in headers.entries) {
    final key = entry.key.toLowerCase();
    if (!RegExp(r"^[!#$%&'*+.^_`|~0-9A-Za-z-]+$").hasMatch(entry.key) ||
        {
          'host',
          'content-length',
          'connection',
          'transfer-encoding',
        }.contains(key) ||
        result.containsKey(key) ||
        entry.value.contains('\r') ||
        entry.value.contains('\n') ||
        entry.value.contains('\x00')) {
      throw ConfigLoadException(
        '$path.${entry.key} is not a valid custom header',
      );
    }
    result[key] = entry.value;
  }
  return result;
}

ModelOptions _options(
  Map<String, Object?> model,
  String path,
  String api,
  String? defaultThinking,
) {
  final compat = _map(model['compat'] ?? {}, '$path.compat');
  final allowed = switch (api) {
    'openai-completions' => {
      'supportsStore',
      'supportsDeveloperRole',
      'supportsReasoningEffort',
      'supportsUsageInStreaming',
      'supportsFinishReason',
      'maxTokensField',
      'requiresToolResultName',
      'requiresAssistantAfterToolResult',
      'requiresThinkingAsText',
      'requiresReasoningContentOnAssistantMessages',
      'thinkingFormat',
      'supportsStrictMode',
      'sendSessionAffinityHeaders',
      'sessionAffinityFormat',
    },
    'openai-responses' => {
      'supportsDeveloperRole',
      'supportsMaxOutputTokens',
      'supportsStrictMode',
      'sessionAffinityFormat',
    },
    'anthropic-messages' => {
      'supportsCacheControlOnTools',
      'supportsTemperature',
      'forceAdaptiveThinking',
      'allowEmptySignature',
      'supportsStrictTools',
      'sendSessionAffinityHeaders',
    },
    _ => <String>{},
  };
  _keys(compat, allowed, '$path.compat');
  for (final entry in compat.entries) {
    final values = switch (entry.key) {
      'maxTokensField' => {'max_completion_tokens', 'max_tokens'},
      'thinkingFormat' => {'openai', 'deepseek'},
      'sessionAffinityFormat' => {'openai', 'openai-nosession', 'openrouter'},
      _ => null,
    };
    if (values != null) {
      if (!values.contains(entry.value)) {
        throw ConfigLoadException('$path.compat.${entry.key} is unsupported');
      }
    } else {
      _boolean(entry.value, '$path.compat.${entry.key}', false);
    }
  }
  final thinking = _map(
    model['thinkingLevelMap'] ?? {},
    '$path.thinkingLevelMap',
  );
  _keys(thinking, _thinkingLevels, '$path.thinkingLevelMap');
  final thinkingMap = thinking.map(
    (key, value) =>
        MapEntry(key, _optionalString(value, '$path.thinkingLevelMap.$key')),
  );
  final sampling = _sampling(model['samplingParams'], '$path.samplingParams');
  final levels = _map(
    model['samplingParamsByThinkingLevel'] ?? {},
    '$path.samplingParamsByThinkingLevel',
  );
  _keys(levels, _thinkingLevels, '$path.samplingParamsByThinkingLevel');
  final byLevel = levels.map(
    (key, value) => MapEntry(
      key,
      _sampling(value, '$path.samplingParamsByThinkingLevel.$key'),
    ),
  );
  if (api == 'anthropic-messages' &&
      (sampling.isNotEmpty || byLevel.isNotEmpty)) {
    throw ConfigLoadException(
      '$path: samplingParams are only implemented for OpenAI APIs',
    );
  }
  final cost = _map(model['cost'] ?? {}, '$path.cost');
  _keys(cost, {
    'input',
    'output',
    'cacheRead',
    'cacheWrite',
    'tiers',
  }, '$path.cost');
  for (final entry in cost.entries.where((e) => e.key != 'tiers')) {
    final amount = _number(entry.value, '$path.cost.${entry.key}');
    if (amount != null && amount < 0) {
      throw ConfigLoadException('$path.cost.${entry.key} must not be negative');
    }
  }
  if (cost['tiers'] != null) {
    for (final tier in _list(cost['tiers'], '$path.cost.tiers')) {
      final map = _map(tier, '$path.cost.tiers');
      _keys(map, {
        'inputTokensAbove',
        'input',
        'output',
        'cacheRead',
        'cacheWrite',
      }, '$path.cost.tiers');
      for (final entry in map.entries) {
        final value = _number(entry.value, '$path.cost.tiers.${entry.key}');
        if (value == null || value < 0) {
          throw ConfigLoadException(
            '$path.cost.tiers contains invalid pricing',
          );
        }
      }
    }
  }
  return ModelOptions(
    compat: Map.unmodifiable(compat),
    thinkingLevelMap: Map.unmodifiable(thinkingMap),
    samplingParams: Map.unmodifiable(sampling),
    samplingParamsByThinkingLevel: Map.unmodifiable(byLevel),
    reasoning: _boolean(model['reasoning'], '$path.reasoning', false),
    defaultThinkingLevel: _clampThinking(defaultThinking, thinkingMap.keys),
    cost: Map.unmodifiable(cost),
  );
}

String? _clampThinking(String? level, Iterable<String> declared) {
  if (level == null) return null;
  final supported = declared.isEmpty
      ? ['off', 'minimal', 'low', 'medium', 'high']
      : declared.toList();
  if (supported.contains(level)) return level;
  final order = _thinkingLevels.toList();
  final lower =
      supported
          .where(
            (candidate) => order.indexOf(candidate) <= order.indexOf(level),
          )
          .toList()
        ..sort((a, b) => order.indexOf(a).compareTo(order.indexOf(b)));
  return lower.lastOrNull ?? supported.first;
}

Map<String, Object?> _sampling(Object? raw, String path) {
  final values = _map(raw ?? {}, path);
  // Sampling options cannot replace routing, messages, tools, or streaming.
  _keys(values, {
    'temperature',
    'top_p',
    'top_k',
    'min_p',
    'frequency_penalty',
    'presence_penalty',
    'repetition_penalty',
    'seed',
  }, path);
  for (final entry in values.entries) {
    _number(entry.value, '$path.${entry.key}');
  }
  return values;
}

import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:path/path.dart' as p;

import 'catalog_snapshot.dart';
import 'json_document.dart';

/// Atlas-verified protocol and endpoint presets. Catalog data cannot change them.
const providerPresets =
    <String, ({String api, String baseUrl, List<String> env})>{
      'openai': (
        api: 'openai-responses',
        baseUrl: 'https://api.openai.com/v1',
        env: ['OPENAI_API_KEY'],
      ),
      'anthropic': (
        api: 'anthropic-messages',
        baseUrl: 'https://api.anthropic.com/v1',
        env: ['ANTHROPIC_API_KEY'],
      ),
    };

// Effort is independent of thinking mode (notably on Opus 4.5). Keep this
// verified first-party list separate from remote reasoning controls. Unknown
// models require a verified preset update or an explicit user override.
const _adaptiveAnthropicModels = {
  'claude-opus-4-6',
  'claude-opus-4-7',
  'claude-opus-4-8',
  'claude-opus-5',
  'claude-opus-5-5',
  'claude-sonnet-4-6',
  'claude-sonnet-5',
  'claude-sonnet-5-5',
  'claude-fable-5',
  'claude-fable-5-1',
};

void _validateCost(Object? value, {bool tier = false}) {
  if (value is! Map) throw const FormatException('Invalid catalog cost');
  for (final key in ['input', 'output', 'cache_read', 'cache_write']) {
    final rate = value[key];
    if ((tier && (key == 'input' || key == 'output') && rate == null) ||
        (rate != null && (rate is! num || !rate.isFinite || rate < 0))) {
      throw const FormatException('Invalid catalog price');
    }
  }
  final tiers = value['tiers'];
  if (tiers != null) {
    if (tiers is! List) throw const FormatException('Invalid catalog tiers');
    for (final entry in tiers) {
      if (entry is! Map || entry['tier'] is! Map) {
        throw const FormatException('Invalid catalog tier');
      }
      final size = (entry['tier'] as Map)['size'];
      if (size is! num || !size.isFinite || size < 0) {
        throw const FormatException('Invalid catalog tier threshold');
      }
      _validateCost(entry, tier: true);
    }
  }
}

/// Model metadata in Pi's model-definition shape, grouped by provider.
final class ModelCatalog(Map<String, List<Map<String, Object?>>> providers) {
  /// Creates a snapshot of normalized model definitions.
  this
    : providers = Map.unmodifiable({
        for (final entry in providers.entries)
          entry.key: List<Map<String, Object?>>.unmodifiable([
            for (final model in entry.value)
              Map<String, Object?>.unmodifiable(model),
          ]),
      });

  /// Supported built-in providers and their model definitions.
  final Map<String, List<Map<String, Object?>>> providers;

  /// The bundled offline snapshot generated from models.dev.
  factory bundled() =>
      ModelCatalog.fromModelsDev(decodeJsonDocument(bundledCatalogJson));

  /// Normalizes supported chat models; endpoint and API selection stay local.
  factory fromModelsDev(Map<String, Object?> document) {
    final result = <String, List<Map<String, Object?>>>{};
    for (final preset in providerPresets.entries) {
      final provider = document[preset.key];
      if (provider is! Map || provider['models'] is! Map) {
        throw FormatException('Missing models.dev provider ${preset.key}');
      }
      final models = <Map<String, Object?>>[];
      for (final entry in (provider['models'] as Map).entries) {
        final id = entry.key;
        if (id is! String || id.isEmpty) {
          throw const FormatException('Invalid catalog model ID');
        }
        final raw = entry.value;
        if (raw is! Map) throw const FormatException('Invalid catalog model');
        if (raw['tool_call'] != true ||
            raw['status'] == 'deprecated' ||
            raw['type'] != null) {
          continue;
        }
        final limits = raw['limit'];
        final modalities = raw['modalities'];
        if (limits is! Map || modalities is! Map) continue;
        final context = limits['context'];
        final output = limits['output'];
        if (context is! int || context <= 0 || output is! int || output <= 0) {
          continue;
        }
        if (modalities['output'] is! List ||
            !(modalities['output'] as List).contains('text')) {
          continue;
        }
        final name = raw['name'] ?? id;
        if (name is! String || name.isEmpty) {
          throw const FormatException('Invalid catalog model name');
        }
        final reasoning = raw['reasoning'] == true;
        final thinking = <String, String?>{};
        final adaptive =
            preset.key == 'anthropic' &&
            _adaptiveAnthropicModels.contains(
              id.replaceFirst(RegExp(r'-\d{8}$'), ''),
            );
        if (reasoning && raw['reasoning_options'] is List) {
          for (final option in raw['reasoning_options'] as List) {
            if (option is Map &&
                option['type'] == 'effort' &&
                option['values'] is List) {
              for (final level in option['values'] as List) {
                if (level is String &&
                    {
                      'none',
                      'minimal',
                      'low',
                      'medium',
                      'high',
                      'xhigh',
                      'max',
                    }.contains(level)) {
                  thinking[level == 'none' ? 'off' : level] = level;
                }
              }
            }
          }
        }
        final cost = raw['cost'];
        if (cost != null) _validateCost(cost);
        models.add({
          'id': id,
          'name': name,
          'api': preset.value.api,
          'baseUrl': preset.value.baseUrl,
          'contextWindow': context,
          'maxTokens': output,
          'reasoning': reasoning,
          if (thinking.isNotEmpty) 'thinkingLevelMap': thinking,
          'input': [
            'text',
            if (modalities['input'] is List &&
                (modalities['input'] as List).contains('image'))
              'image',
          ],
          if (cost is Map)
            'cost': {
              'input': cost['input'],
              'output': cost['output'],
              'cacheRead': cost['cache_read'],
              'cacheWrite': cost['cache_write'],
              if (cost['tiers'] is List)
                'tiers': [
                  for (final tier in cost['tiers'] as List)
                    if (tier is Map &&
                        tier['tier'] is Map &&
                        (tier['tier'] as Map)['size'] is num)
                      {
                        'inputTokensAbove': (tier['tier'] as Map)['size'],
                        'input': tier['input'],
                        'output': tier['output'],
                        if (tier['cache_read'] != null)
                          'cacheRead': tier['cache_read'],
                        if (tier['cache_write'] != null)
                          'cacheWrite': tier['cache_write'],
                      },
                ],
            },
          'compat': {
            if (preset.key == 'anthropic') ...{
              'supportsTemperature': raw['temperature'] != false,
              'forceAdaptiveThinking': adaptive,
            },
          },
        });
      }
      if (models.isEmpty) {
        throw FormatException('Empty catalog for ${preset.key}');
      }
      result[preset.key] = models;
    }
    return ModelCatalog(result);
  }

  /// Restores a valid cache, falling back to the bundled snapshot when damaged.
  static ModelCatalog load(Directory atlasDirectory) {
    final file = File(
      p.join(atlasDirectory.path, 'cache', 'model-catalog.json'),
    );
    if (file.existsSync()) {
      try {
        final envelope = decodeJsonDocument(file.readAsStringSync());
        if (envelope['version'] == 1 &&
            envelope['data'] is Map<String, Object?>) {
          return ModelCatalog.fromModelsDev(
            envelope['data'] as Map<String, Object?>,
          );
        }
      } on Object {
        // A downloadable cache must never make offline startup impossible.
      }
    }
    return ModelCatalog.bundled();
  }

  /// Downloads and validates a replacement before atomically publishing it.
  static Future<ModelCatalog> refresh(
    Directory atlasDirectory, {
    Dio? client,
  }) async {
    final http =
        client ??
        Dio(
          BaseOptions(
            connectTimeout: const Duration(seconds: 10),
            receiveTimeout: const Duration(seconds: 30),
          ),
        );
    Directory? staging;
    try {
      final response = await http.get<Object?>('https://models.dev/api.json');
      final data = response.data;
      if (data is! Map<String, Object?>) {
        throw const FormatException('Invalid models.dev response');
      }
      final catalog = ModelCatalog.fromModelsDev(data);
      final cache = Directory(p.join(atlasDirectory.path, 'cache'));
      await cache.create(recursive: true);
      staging = await cache.createTemp('.catalog-');
      final temp = File(p.join(staging.path, 'catalog.json'));
      await temp.writeAsString(
        jsonEncode({
          'version': 1,
          'source': 'https://models.dev/api.json',
          'fetchedAt': DateTime.now().toUtc().toIso8601String(),
          'data': data,
        }),
        flush: true,
      );
      await temp.rename(p.join(cache.path, 'model-catalog.json'));
      return catalog;
    } finally {
      if (client == null) http.close();
      await staging?.delete(recursive: true);
    }
  }
}

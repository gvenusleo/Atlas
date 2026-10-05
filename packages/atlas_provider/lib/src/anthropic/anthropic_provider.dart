import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:dio/dio.dart';

import '../http_stream_client.dart';
import '../json_utils.dart';
import '../stream_runner.dart';
import 'anthropic_configuration.dart';
import 'anthropic_parser.dart';

/// Streams configured models through the Anthropic Messages API.
final class AnthropicProvider(
  List<AnthropicProviderConfiguration> configurations, {
  HttpStreamClient? httpClient,
}) implements ModelProvider {
  /// Creates a provider from endpoint configurations and an optional HTTP client.
  this
    : _entries = _indexConfigurations(configurations),
      _httpClient = httpClient ?? DioHttpStreamClient();

  final Map<ModelRef, _ModelEntry> _entries;
  final HttpStreamClient _httpClient;

  /// Returns the configured descriptor for [model].
  @override
  Future<ModelDescriptor> describe(ModelRef model) async {
    final entry = _entries[model];
    if (entry == null) {
      throw ArgumentError.value(model, 'model', 'is not configured');
    }
    return entry.configuration.descriptor;
  }

  /// Streams one configured model step and emits exactly one terminal event.
  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) {
    final entry = _entries[request.model];
    if (entry == null) {
      return notFoundStream(request.model);
    }
    return runModelStream(
      request: request,
      openStream: () {
        validateRequestCapabilities(
          request,
          entry.configuration.descriptor,
          (message) => AnthropicProviderException(
            providerId: entry.provider.id,
            message: message,
          ),
        );
        return _openStream(request, entry);
      },
      createParser: () => AnthropicParser(entry.provider.id),
      toFailure: (error) => switch (error) {
        DioException() =>
          request.cancellation?.isCancelled == true
              ? const TurnCancelledException()
              : AnthropicProviderException(
                  providerId: entry.provider.id,
                  message: 'stream failed after the response started',
                ),
        HttpStreamException(:final statusCode, :final detail) =>
          AnthropicProviderException(
            providerId: entry.provider.id,
            message: statusCode == null
                ? 'provider request failed'
                : 'provider request failed (status $statusCode)',
            statusCode: statusCode,
            detail: detail,
          ),
        FormatException() => AnthropicProviderException(
          providerId: entry.provider.id,
          message: 'stream returned malformed data',
        ),
        _ => error,
      },
    );
  }

  Future<ActiveHttpStream> _openStream(
    ModelRequest request,
    _ModelEntry entry,
  ) async {
    final auth = await entry.provider.authentication?.resolve();
    request.cancellation?.throwIfCancelled();
    final uri = entry.provider.baseUrl.replace(
      path:
          '${entry.provider.baseUrl.path.replaceFirst(RegExp(r'/+$'), '')}/messages',
    );
    final headers = <String, Object>{
      Headers.contentTypeHeader: Headers.jsonContentType,
      Headers.acceptHeader: 'text/event-stream',
      if ((auth?.key ?? entry.provider.apiKey).isNotEmpty)
        'x-api-key': auth?.key ?? entry.provider.apiKey,
      'anthropic-version': entry.provider.apiVersion,
      'user-agent': entry.provider.userAgent ?? 'Atlas',
    };
    if (entry.configuration.options.flag('sendSessionAffinityHeaders', false)) {
      headers['x-session-affinity'] = request.sessionId.value;
    }
    if (auth != null) headers.addAll(auth.headers);
    return _httpClient.openStream(
      uri: uri,
      body: _anthropicRequest(request, entry),
      headers: headers,
      cancellation: request.cancellation,
    );
  }
}

final class const _ModelEntry(
  final AnthropicProviderConfiguration provider,
  final AnthropicModelConfiguration configuration,
) {}

Map<ModelRef, _ModelEntry> _indexConfigurations(
  List<AnthropicProviderConfiguration> configurations,
) {
  if (configurations.isEmpty) {
    throw ArgumentError.value(configurations, 'configurations', 'is empty');
  }
  final entries = <ModelRef, _ModelEntry>{};
  final providers = <ProviderId>{};
  for (final provider in configurations) {
    if (!providers.add(provider.id)) {
      throw ArgumentError('duplicate provider: ${provider.id}');
    }
    if ((provider.baseUrl.scheme != 'http' &&
            provider.baseUrl.scheme != 'https') ||
        provider.baseUrl.host.isEmpty ||
        provider.baseUrl.hasQuery ||
        provider.baseUrl.hasFragment) {
      throw ArgumentError.value(
        provider.baseUrl,
        'baseUrl',
        'must be an HTTP URL without a query or fragment',
      );
    }
    if (provider.models.isEmpty) {
      throw ArgumentError('provider ${provider.id} has no models');
    }
    for (final model in provider.models) {
      if (model.descriptor.ref.providerId != provider.id) {
        throw ArgumentError(
          'model ${model.descriptor.ref} belongs to another provider',
        );
      }
      if (entries.containsKey(model.descriptor.ref)) {
        throw ArgumentError('duplicate model: ${model.descriptor.ref}');
      }
      entries[model.descriptor.ref] = _ModelEntry(provider, model);
    }
  }
  return Map<ModelRef, _ModelEntry>.unmodifiable(entries);
}

Map<String, Object?> _anthropicRequest(
  ModelRequest request,
  _ModelEntry entry,
) {
  final descriptor = entry.configuration.descriptor;
  final options = entry.configuration.options;
  final level = options.level(request.reasoningEffort);
  final effort = options.effort(level);
  final maxTokens = request.maxOutputTokens > 0
      ? request.maxOutputTokens
      : (descriptor.maxOutputTokens > 0
            ? descriptor.maxOutputTokens.clamp(1, 4096)
            : 4096);
  if (descriptor.maxOutputTokens > 0 &&
      maxTokens > descriptor.maxOutputTokens) {
    throw AnthropicProviderException(
      providerId: entry.provider.id,
      message: 'output budget exceeds model maxTokens',
    );
  }
  final adaptive = options.flag('forceAdaptiveThinking', false);
  final manualThinking =
      !adaptive &&
      level != null &&
      level != 'off' &&
      (options.reasoning || entry.configuration.thinkingBudgetTokens > 0);
  var thinkingBudget = 0;
  if (manualThinking) {
    // Manual thinking needs at least 1024 tokens; keep another 1024 available
    // for the answer under the shared max_tokens ceiling.
    if (maxTokens < 2048) {
      throw AnthropicProviderException(
        providerId: entry.provider.id,
        message:
            'manual thinking requires an output budget of at least 2048 tokens',
      );
    }
    final requestedBudget = entry.configuration.thinkingBudgetTokens > 0
        ? _thinkingBudget(entry.configuration.thinkingBudgetTokens, level)
        : switch (level) {
            'minimal' => 1024,
            'low' => 2048,
            'medium' => 8192,
            _ => 16384,
          };
    if (requestedBudget < 1024) {
      throw AnthropicProviderException(
        providerId: entry.provider.id,
        message: 'manual thinking budget must be at least 1024 tokens',
      );
    }
    thinkingBudget = requestedBudget.clamp(1024, maxTokens - 1024);
  }
  final thinkingEnabled =
      level != null && level != 'off' && (manualThinking || adaptive);
  final messages = _anthropicMessages(
    request.messages,
    allowEmptySignature: options.flag('allowEmptySignature', false),
  );
  // A breakpoint on the last message extends the cached prefix by one step per
  // request instead of re-writing the whole conversation each time.
  _markTrailingCacheBreakpoint(messages);
  final result = <String, Object?>{
    'model': descriptor.ref.modelId.value,
    'max_tokens': maxTokens,
    'messages': messages,
    'stream': true,
  };
  if (request.systemPrompt.isNotEmpty) {
    result['system'] = <Object?>[
      <String, Object?>{
        'type': 'text',
        'text': request.systemPrompt,
        'cache_control': _ephemeralCache,
      },
    ];
  }
  final tools = _tools(request.tools);
  if (tools.isNotEmpty) {
    // The tool block precedes the system prompt, so marking it caches the
    // whole preamble that every request in the session shares.
    if (options.flag('supportsCacheControlOnTools', true)) {
      (tools.last as Map<String, Object?>)['cache_control'] = _ephemeralCache;
    }
    if (options.flag('supportsStrictTools', false)) {
      for (final tool in tools.cast<Map<String, Object?>>()) {
        final schema = tool['input_schema'];
        if (schema is Map &&
            schema['type'] == 'object' &&
            _supportsStrictSchema(schema)) {
          tool['strict'] = true;
        }
      }
    }
    result['tools'] = tools;
  }
  if (!thinkingEnabled &&
      request.temperature != null &&
      options.flag('supportsTemperature', true)) {
    result['temperature'] = request.temperature;
  }
  if (thinkingEnabled) {
    result['thinking'] = adaptive
        ? <String, Object?>{'type': 'adaptive'}
        : <String, Object?>{'type': 'enabled', 'budget_tokens': thinkingBudget};
    if (effort != null && (adaptive || options.thinkingLevelMap.isNotEmpty)) {
      result['output_config'] = {'effort': effort};
    }
  }
  return result;
}

// Strict sampling accepts a smaller schema language than ordinary tool use.
// Preserve schemas unchanged and fall back to ordinary tool use when a schema
// needs unsupported constraints, references, or open objects.
bool _supportsStrictSchema(Map<Object?, Object?> schema) {
  const keywords = {
    'type',
    'description',
    'title',
    'properties',
    'required',
    'additionalProperties',
    'items',
    'enum',
    'const',
    'anyOf',
    'allOf',
  };
  if (schema.keys.any((key) => !keywords.contains(key))) return false;
  for (final key in ['description', 'title']) {
    if (schema.containsKey(key) && schema[key] is! String) return false;
  }
  const types = {
    'object',
    'array',
    'string',
    'integer',
    'number',
    'boolean',
    'null',
  };
  final rawType = schema['type'];
  final declaredTypes = rawType is List ? rawType : [?rawType];
  if (declaredTypes.any((type) => !types.contains(type))) return false;
  if (declaredTypes.contains('object')) {
    if (schema['additionalProperties'] != false) return false;
    final properties = schema['properties'] ?? const <String, Object?>{};
    if (properties is! Map ||
        properties.values.any(
          (value) => value is! Map || !_supportsStrictSchema(value),
        )) {
      return false;
    }
    final required = schema['required'];
    if (required != null &&
        (required is! List ||
            required.any(
              (name) => name is! String || !properties.containsKey(name),
            ))) {
      return false;
    }
  } else if (schema.containsKey('properties') ||
      schema.containsKey('additionalProperties') ||
      schema.containsKey('required')) {
    return false;
  }
  if (declaredTypes.contains('array')) {
    final items = schema['items'];
    if (items is! Map || !_supportsStrictSchema(items)) return false;
  } else if (schema.containsKey('items')) {
    return false;
  }
  bool scalar(Object? value) =>
      value == null ||
      value is String ||
      value is bool ||
      (value is num && value.isFinite);
  if (schema.containsKey('const') && !scalar(schema['const'])) return false;
  if (schema.containsKey('enum')) {
    final values = schema['enum'];
    if (values is! List ||
        values.isEmpty ||
        values.any((value) => !scalar(value))) {
      return false;
    }
  }
  for (final key in ['anyOf', 'allOf']) {
    if (!schema.containsKey(key)) continue;
    final branches = schema[key];
    if (branches is! List ||
        branches.isEmpty ||
        branches.any(
          (value) => value is! Map || !_supportsStrictSchema(value),
        )) {
      return false;
    }
  }
  return declaredTypes.isNotEmpty ||
      schema.containsKey('enum') ||
      schema.containsKey('const') ||
      schema.containsKey('anyOf') ||
      schema.containsKey('allOf');
}

int _thinkingBudget(int base, String? effort) {
  if (base <= 0 || effort == null) return base;
  final multiplier = switch (effort) {
    'minimal' => 0.25,
    'low' => 0.5,
    'max' => 1.5,
    _ => 1.0,
  };
  return (base * multiplier).round().clamp(1, 1 << 30);
}

List<Object?> _anthropicMessages(
  List<ModelMessage> messages, {
  bool allowEmptySignature = false,
}) {
  final result = <Object?>[];
  for (var index = 0; index < messages.length; index++) {
    final message = messages[index];
    if (message.role == ModelMessageRole.tool) {
      final blocks = <Object?>[];
      while (index < messages.length &&
          messages[index].role == ModelMessageRole.tool) {
        final tool = messages[index];
        blocks.add(<String, Object?>{
          'type': 'tool_result',
          'tool_use_id': tool.toolCallId?.value,
          'content': tool.toolOutput ?? '',
        });
        index++;
      }
      index--;
      result.add(<String, Object?>{'role': 'user', 'content': blocks});
      continue;
    }
    if (message.role == ModelMessageRole.assistant &&
        message.content.isEmpty &&
        message.toolCalls.isEmpty) {
      continue;
    }
    final content = <Object?>[
      ..._replayedThinking(message, allowEmptySignature),
      ..._anthropicContent(message.content),
      for (final call in message.toolCalls)
        <String, Object?>{
          'type': 'tool_use',
          'id': call.id.value,
          'name': call.name,
          'input': call.arguments,
        },
    ];
    result.add(<String, Object?>{
      'role': message.role.name,
      'content': content,
    });
  }
  return result;
}

List<Object?> _replayedThinking(
  ModelMessage message,
  bool allowEmptySignature,
) {
  final blocks = message.continuation?.opaquePayload['thinking_blocks'];
  if (blocks is! List) {
    return const <Object?>[];
  }
  final result = <Object?>[];
  for (final raw in blocks) {
    final block = asJsonMap(raw);
    if (block['type'] == 'redacted_thinking') {
      final data = block['data'];
      if (data is String && data.isNotEmpty) {
        result.add(<String, Object?>{
          'type': 'redacted_thinking',
          'data': data,
        });
      }
      continue;
    }
    final text = block['thinking'];
    final signature = block['signature'];
    if (text is String &&
        signature is String &&
        (signature.isNotEmpty || allowEmptySignature)) {
      result.add(<String, Object?>{
        'type': 'thinking',
        'thinking': text,
        'signature': signature,
      });
    }
  }
  return result;
}

List<Object?> _anthropicContent(List<ContentPart> parts) => parts.map((part) {
  if (part is TextContent) {
    return <String, Object?>{'type': 'text', 'text': part.text};
  }
  if (part is ResourceContent) {
    return <String, Object?>{
      'type': 'document',
      'source': <String, Object?>{
        'type': 'text',
        'media_type': part.mimeType ?? 'text/plain',
        'data': part.text,
      },
    };
  }
  final image = part as ImageContent;
  return <String, Object?>{'type': 'image', 'source': _imageSource(image)};
}).toList();

Object _imageSource(ImageContent image) {
  final source = image.source;
  if (source.startsWith('data:')) {
    final comma = source.indexOf(',');
    if (comma > 5) {
      final mediaType = source.substring(5, comma).split(';').first;
      return <String, Object?>{
        'type': 'base64',
        'media_type': mediaType,
        'data': source.substring(comma + 1),
      };
    }
  }
  return <String, Object?>{'type': 'url', 'url': source};
}

List<Object?> _tools(List<ToolDescriptor> tools) => tools
    .map(
      (tool) => <String, Object?>{
        'name': tool.name,
        'description': tool.description,
        'input_schema': tool.inputSchema,
      },
    )
    .toList();

/// Cache breakpoint marker for Anthropic prompt caching.
const _ephemeralCache = <String, Object?>{'type': 'ephemeral'};

/// Marks the last cacheable block in [messages] as a cache breakpoint.
///
/// Thinking blocks cannot carry `cache_control`, so a trailing assistant turn
/// made only of thinking ends the cached prefix at the last eligible block
/// instead of at the very end of the conversation.
void _markTrailingCacheBreakpoint(List<Object?> messages) {
  for (var index = messages.length - 1; index >= 0; index--) {
    final message = messages[index];
    if (message is! Map<String, Object?>) {
      continue;
    }
    final content = message['content'];
    if (content is! List) {
      continue;
    }
    for (var block = content.length - 1; block >= 0; block--) {
      final entry = content[block];
      if (entry is! Map<String, Object?>) {
        continue;
      }
      final type = entry['type'];
      if (type == 'thinking' || type == 'redacted_thinking') {
        continue;
      }
      entry['cache_control'] = _ephemeralCache;
      return;
    }
  }
}

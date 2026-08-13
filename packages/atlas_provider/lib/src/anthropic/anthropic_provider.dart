import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:dio/dio.dart';

import '../http_stream_client.dart';
import '../json_utils.dart';
import '../stream_runner.dart';
import 'anthropic_configuration.dart';
import 'anthropic_parser.dart';

/// Streams configured models through the Anthropic Messages API.
final class AnthropicProvider implements ModelProvider {
  /// Creates a provider from endpoint configurations and an optional HTTP client.
  AnthropicProvider(
    List<AnthropicProviderConfiguration> configurations, {
    HttpStreamClient? httpClient,
  }) : _entries = _indexConfigurations(configurations),
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
        HttpStreamException(:final message, :final statusCode) =>
          AnthropicProviderException(
            providerId: entry.provider.id,
            message: message,
            statusCode: statusCode,
          ),
        _ => error,
      },
    );
  }

  Future<ActiveHttpStream> _openStream(
    ModelRequest request,
    _ModelEntry entry,
  ) {
    final uri = entry.provider.baseUrl.replace(
      path:
          '${entry.provider.baseUrl.path.replaceFirst(RegExp(r'/+$'), '')}/v1/messages',
    );
    final headers = <String, Object>{
      Headers.contentTypeHeader: Headers.jsonContentType,
      Headers.acceptHeader: 'text/event-stream',
      'x-api-key': entry.provider.apiKey,
      'anthropic-version': entry.provider.apiVersion,
      'user-agent': entry.provider.userAgent ?? 'Atlas',
    };
    return _httpClient.openStream(
      uri: uri,
      body: _anthropicRequest(request, entry),
      headers: headers,
      secret: entry.provider.apiKey,
      cancellation: request.cancellation,
    );
  }
}

final class _ModelEntry {
  const _ModelEntry(this.provider, this.configuration);

  final AnthropicProviderConfiguration provider;
  final AnthropicModelConfiguration configuration;
}

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
  final result = <String, Object?>{
    'model': descriptor.ref.modelId.value,
    'max_tokens': request.maxOutputTokens > 0
        ? request.maxOutputTokens
        : (descriptor.maxOutputTokens > 0 ? descriptor.maxOutputTokens : 4096),
    'messages': _anthropicMessages(request.messages),
    'stream': true,
  };
  if (request.systemPrompt.isNotEmpty) {
    result['system'] = request.systemPrompt;
  }
  final tools = _tools(request.tools);
  if (tools.isNotEmpty) {
    result['tools'] = tools;
  }
  if (request.temperature != null) {
    result['temperature'] = request.temperature;
  }
  if (request.reasoningEffort != null &&
      entry.configuration.thinkingBudgetTokens > 0) {
    result['thinking'] = <String, Object?>{
      'type': 'enabled',
      'budget_tokens': entry.configuration.thinkingBudgetTokens,
    };
  }
  return result;
}

List<Object?> _anthropicMessages(List<ModelMessage> messages) {
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
      ..._replayedThinking(message),
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

List<Object?> _replayedThinking(ModelMessage message) {
  final blocks = message.continuation?.opaquePayload['thinking_blocks'];
  if (blocks is! List) {
    return const <Object?>[];
  }
  final result = <Object?>[];
  for (final raw in blocks) {
    final block = asJsonMap(raw);
    final text = block['thinking'];
    final signature = block['signature'];
    if (text is String &&
        text.isNotEmpty &&
        signature is String &&
        signature.isNotEmpty) {
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

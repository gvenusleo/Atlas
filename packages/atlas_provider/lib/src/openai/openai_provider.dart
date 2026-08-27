import 'dart:async';
import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:dio/dio.dart';

import '../http_stream_client.dart';
import '../stream_runner.dart';
import 'chat_parser.dart';
import 'openai_configuration.dart';
import 'responses_parser.dart';

/// Streams configured models through the OpenAI Chat Completions or Responses API.
final class OpenAICompatibleProvider implements ModelProvider {
  /// Creates a provider from endpoint configurations and an optional HTTP client.
  OpenAICompatibleProvider(
    List<OpenAIProviderConfiguration> configurations, {
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
          (message) => OpenAIProviderException(
            providerId: entry.provider.id,
            message: message,
          ),
        );
        return _openStream(request, entry);
      },
      createParser: () => entry.provider.protocol == OpenAIProtocol.responses
          ? ResponsesParser(
              entry.provider.id,
              entry.configuration.descriptor.ref.modelId.value,
              request.maxOutputTokens > 0,
            )
          : ChatParser(entry.provider.id),
      toFailure: (error) => switch (error) {
        DioException() =>
          request.cancellation?.isCancelled == true
              ? const TurnCancelledException()
              : OpenAIProviderException(
                  providerId: entry.provider.id,
                  message: 'stream failed after the response started',
                ),
        HttpStreamException(:final statusCode, :final detail) =>
          OpenAIProviderException(
            providerId: entry.provider.id,
            message: statusCode == null
                ? 'provider request failed'
                : 'provider request failed (status $statusCode)',
            statusCode: statusCode,
            detail: detail,
          ),
        FormatException() => OpenAIProviderException(
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
  ) {
    final uri = entry.provider.baseUrl.replace(
      path:
          '${entry.provider.baseUrl.path.replaceFirst(RegExp(r'/+$'), '')}${entry.provider.protocol == OpenAIProtocol.responses ? '/responses' : '/chat/completions'}',
    );
    final headers = <String, Object>{
      Headers.contentTypeHeader: Headers.jsonContentType,
      Headers.acceptHeader: 'text/event-stream',
      'user-agent': entry.provider.userAgent ?? 'Atlas',
    };
    if (entry.provider.apiKey.isNotEmpty) {
      headers['authorization'] = 'Bearer ${entry.provider.apiKey}';
    }
    return _httpClient.openStream(
      uri: uri,
      body: entry.provider.protocol == OpenAIProtocol.responses
          ? _responsesRequest(request, entry)
          : _chatRequest(request, entry),
      headers: headers,
      cancellation: request.cancellation,
    );
  }
}

final class _ModelEntry {
  const _ModelEntry(this.provider, this.configuration);

  final OpenAIProviderConfiguration provider;
  final OpenAIModelConfiguration configuration;
}

Map<ModelRef, _ModelEntry> _indexConfigurations(
  List<OpenAIProviderConfiguration> configurations,
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

Map<String, Object?> _chatRequest(ModelRequest request, _ModelEntry entry) {
  final result = <String, Object?>{
    'model': entry.configuration.descriptor.ref.modelId.value,
    'messages': _chatMessages(request.messages, request.systemPrompt),
    'stream': true,
    'stream_options': <String, Object?>{'include_usage': true},
  };
  result.addAll(request.providerOptions);
  final tools = _tools(request.tools, responses: false);
  if (tools.isNotEmpty) {
    result['tools'] = tools;
  }
  if (request.maxOutputTokens > 0) {
    result['max_completion_tokens'] = request.maxOutputTokens;
  }
  if (request.temperature != null && request.reasoningEffort == null) {
    result['temperature'] = request.temperature;
  }
  if (request.reasoningEffort != null) {
    result['reasoning_effort'] = request.reasoningEffort;
  }
  if (entry.configuration.promptCacheEnabled) {
    result['prompt_cache_key'] = request.sessionId.value;
  }
  return result;
}

Map<String, Object?> _responsesRequest(
  ModelRequest request,
  _ModelEntry entry,
) {
  final result = <String, Object?>{
    'model': entry.configuration.descriptor.ref.modelId.value,
    'input': _responsesInput(
      request.messages,
      entry.provider.id,
      entry.configuration.descriptor.ref.modelId.value,
    ),
    'stream': true,
  };
  result.addAll(request.providerOptions);
  if (request.systemPrompt.isNotEmpty) {
    result['instructions'] = request.systemPrompt;
  }
  final tools = _tools(request.tools, responses: true);
  if (tools.isNotEmpty) {
    result['tools'] = tools;
  }
  if (request.maxOutputTokens > 0) {
    result['max_output_tokens'] = request.maxOutputTokens;
  }
  if (request.temperature != null && request.reasoningEffort == null) {
    result['temperature'] = request.temperature;
  }
  if (request.reasoningEffort != null) {
    result['reasoning'] = <String, Object?>{'effort': request.reasoningEffort};
  }
  if (entry.configuration.promptCacheEnabled) {
    result['prompt_cache_key'] = request.sessionId.value;
  }
  return result;
}

List<Object?> _chatMessages(List<ModelMessage> messages, String systemPrompt) {
  final result = <Object?>[];
  if (systemPrompt.isNotEmpty) {
    result.add(<String, Object?>{'role': 'system', 'content': systemPrompt});
  }
  for (final message in messages) {
    if (message.role == ModelMessageRole.assistant &&
        message.content.isEmpty &&
        message.toolCalls.isEmpty) {
      continue;
    }
    final item = <String, Object?>{'role': message.role.name};
    if (message.role == ModelMessageRole.tool) {
      item['content'] = message.toolOutput ?? '';
      item['tool_call_id'] = message.toolCallId?.value;
    } else {
      item['content'] = message.content.isEmpty
          ? ''
          : _chatContent(message.content);
      if (message.role == ModelMessageRole.assistant &&
          message.continuation?.reasoningSummary.isNotEmpty == true) {
        item['reasoning_content'] = message.continuation!.reasoningSummary;
      }
      if (message.toolCalls.isNotEmpty) {
        item['tool_calls'] = message.toolCalls
            .map(
              (call) => <String, Object?>{
                'id': call.id.value,
                'type': 'function',
                'function': <String, Object?>{
                  'name': call.name,
                  'arguments': jsonEncode(call.arguments),
                },
              },
            )
            .toList();
      }
    }
    result.add(item);
  }
  return result;
}

Object _chatContent(List<ContentPart> parts) {
  if (parts.every((part) => part is TextContent)) {
    return parts.whereType<TextContent>().map((part) => part.text).join();
  }
  return parts.map((part) {
    if (part is TextContent) {
      return <String, Object?>{'type': 'text', 'text': part.text};
    }
    if (part is ResourceContent) {
      // OpenAI-compatible APIs have no resource block; embed the text as a
      // plain text part.
      return <String, Object?>{'type': 'text', 'text': part.text};
    }
    final image = part as ImageContent;
    return <String, Object?>{
      'type': 'image_url',
      'image_url': <String, Object?>{
        'url': image.source,
        'detail': image.detail.name,
      },
    };
  }).toList();
}

List<Object?> _responsesInput(
  List<ModelMessage> messages,
  ProviderId providerId,
  String modelId,
) {
  final result = <Object?>[];
  for (final message in messages) {
    if (message.role == ModelMessageRole.assistant &&
        message.content.isEmpty &&
        message.toolCalls.isEmpty &&
        message.continuation == null) {
      continue;
    }
    if (message.role == ModelMessageRole.assistant &&
        message.continuation?.providerId == providerId &&
        message.continuation?.opaquePayload['protocol'] == 'responses' &&
        message.continuation?.opaquePayload['model'] == modelId) {
      final items = message.continuation!.opaquePayload['items'];
      if (items is List && items.isNotEmpty) {
        result.addAll(items.cast<Object?>());
        continue;
      }
    }
    if (message.role == ModelMessageRole.tool) {
      result.add(<String, Object?>{
        'type': 'function_call_output',
        'call_id': message.toolCallId?.value,
        'output': message.toolOutput ?? '',
      });
      continue;
    }
    if (message.role == ModelMessageRole.assistant &&
        message.toolCalls.isNotEmpty) {
      if (message.content.isNotEmpty) {
        result.add(<String, Object?>{
          'role': 'assistant',
          'content': _responsesContent(message.content),
        });
      }
      for (final call in message.toolCalls) {
        result.add(<String, Object?>{
          'type': 'function_call',
          'call_id': call.id.value,
          'name': call.name,
          'arguments': jsonEncode(call.arguments),
        });
      }
      continue;
    }
    result.add(<String, Object?>{
      'role': message.role.name,
      'content': _responsesContent(message.content),
    });
  }
  return result;
}

List<Object?> _responsesContent(List<ContentPart> parts) => parts.map((part) {
  if (part is TextContent) {
    return <String, Object?>{'type': 'input_text', 'text': part.text};
  }
  if (part is ResourceContent) {
    // The Responses API has no resource block; embed the text as plain text.
    return <String, Object?>{'type': 'input_text', 'text': part.text};
  }
  final image = part as ImageContent;
  return <String, Object?>{
    'type': 'input_image',
    'image_url': image.source,
    'detail': image.detail.name,
  };
}).toList();

List<Object?> _tools(List<ToolDescriptor> tools, {required bool responses}) =>
    tools
        .map(
          (tool) => responses
              ? <String, Object?>{
                  'type': 'function',
                  'name': tool.name,
                  'description': tool.description,
                  'parameters': tool.inputSchema,
                  'strict': false,
                }
              : <String, Object?>{
                  'type': 'function',
                  'function': <String, Object?>{
                    'name': tool.name,
                    'description': tool.description,
                    'parameters': tool.inputSchema,
                  },
                },
        )
        .toList();

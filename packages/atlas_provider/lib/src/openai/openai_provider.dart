import 'dart:async';
import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:dio/dio.dart';

import '../http_stream_client.dart';
import '../stream_runner.dart';
import 'chat_parser.dart';
import 'openai_configuration.dart';
import 'responses_parser.dart';
import '../model_options.dart';

/// Streams configured models through the OpenAI Chat Completions or Responses API.
final class OpenAICompatibleProvider(
  List<OpenAIProviderConfiguration> configurations, {
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
              entry.configuration.options.flag(
                    'supportsMaxOutputTokens',
                    true,
                  ) &&
                  (request.maxOutputTokens > 0 ||
                      entry.configuration.descriptor.maxOutputTokens > 0),
            )
          : ChatParser(
              entry.provider.id,
              supportsFinishReason: entry.configuration.options.flag(
                'supportsFinishReason',
                true,
              ),
            ),
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
  ) async {
    final auth = await entry.provider.authentication?.resolve();
    request.cancellation?.throwIfCancelled();
    final uri = entry.provider.baseUrl.replace(
      path:
          '${entry.provider.baseUrl.path.replaceFirst(RegExp(r'/+$'), '')}${entry.provider.protocol == OpenAIProtocol.responses ? '/responses' : '/chat/completions'}',
    );
    final headers = <String, Object>{
      Headers.contentTypeHeader: Headers.jsonContentType,
      Headers.acceptHeader: 'text/event-stream',
      'user-agent': entry.provider.userAgent ?? 'Atlas',
    };
    final options = entry.configuration.options;
    if (options.flag('sendSessionAffinityHeaders', false) ||
        (entry.provider.protocol == OpenAIProtocol.responses &&
            options.compat.containsKey('sessionAffinityFormat'))) {
      final format = options.compat['sessionAffinityFormat'] ?? 'openai';
      if (format == 'openrouter') {
        headers['x-session-id'] = request.sessionId.value;
      } else {
        if (format == 'openai') headers['session_id'] = request.sessionId.value;
        headers['x-client-request-id'] = request.sessionId.value;
        headers['x-session-affinity'] = request.sessionId.value;
      }
    }
    final key = auth?.key ?? entry.provider.apiKey;
    if (key.isNotEmpty) headers['authorization'] = 'Bearer $key';
    if (auth != null) headers.addAll(auth.headers);
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

final class const _ModelEntry(
  final OpenAIProviderConfiguration provider,
  final OpenAIModelConfiguration configuration,
) {}

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
  final options = entry.configuration.options;
  final level = options.level(request.reasoningEffort);
  final effort = options.effort(level);
  final result = <String, Object?>{
    'model': entry.configuration.descriptor.ref.modelId.value,
    'messages': _chatMessages(request.messages, request.systemPrompt, options),
    'stream': true,
    if (options.flag('supportsUsageInStreaming', true))
      'stream_options': <String, Object?>{'include_usage': true},
    if (options.flag('supportsStore', false)) 'store': false,
  };
  result.addAll(options.sampling(level));
  final tools = _tools(request.tools, responses: false);
  if (tools.isNotEmpty) {
    result['tools'] = tools;
  }
  final maxTokens = _outputBudget(request, entry);
  if (maxTokens > 0) {
    result[options.compat['maxTokensField'] as String? ??
            'max_completion_tokens'] =
        maxTokens;
  }
  if (request.temperature != null && effort == null) {
    result['temperature'] = request.temperature;
  }
  if (options.compat['thinkingFormat'] == 'deepseek') {
    if (level != null) {
      result['thinking'] = {'type': level == 'off' ? 'disabled' : 'enabled'};
    }
    if (effort != null && options.flag('supportsReasoningEffort', true)) {
      result['reasoning_effort'] = effort;
    }
  } else if (effort != null && options.flag('supportsReasoningEffort', true)) {
    result['reasoning_effort'] = effort;
  }
  if (options.flag('supportsStrictMode', false)) {
    for (final tool in tools.cast<Map<String, Object?>>()) {
      (tool['function'] as Map<String, Object?>)['strict'] = false;
    }
  }
  if (entry.provider.baseUrl.host == 'api.openai.com') {
    result['prompt_cache_key'] = request.sessionId.value;
  }
  return result;
}

Map<String, Object?> _responsesRequest(
  ModelRequest request,
  _ModelEntry entry,
) {
  final options = entry.configuration.options;
  final level = options.level(request.reasoningEffort);
  final effort = options.effort(level);
  final result = <String, Object?>{
    'model': entry.configuration.descriptor.ref.modelId.value,
    'input': _responsesInput(
      request.messages,
      entry.provider.id,
      entry.configuration.descriptor.ref.modelId.value,
    ),
    'stream': true,
  };
  result.addAll(options.sampling(level));
  if (request.systemPrompt.isNotEmpty) {
    if (options.flag('supportsDeveloperRole', true)) {
      result['instructions'] = request.systemPrompt;
    } else {
      (result['input'] as List<Object?>).insert(0, {
        'role': 'system',
        'content': request.systemPrompt,
      });
    }
  }
  final tools = _tools(request.tools, responses: true);
  if (tools.isNotEmpty) {
    result['tools'] = tools;
  }
  final maxTokens = _outputBudget(request, entry);
  if (maxTokens > 0 && options.flag('supportsMaxOutputTokens', true)) {
    result['max_output_tokens'] = maxTokens;
  }
  if (request.temperature != null && effort == null) {
    result['temperature'] = request.temperature;
  }
  if (effort != null) result['reasoning'] = <String, Object?>{'effort': effort};
  if (!options.flag('supportsStrictMode', true)) {
    for (final tool in tools.cast<Map<String, Object?>>()) {
      tool.remove('strict');
    }
  }
  result['prompt_cache_key'] = request.sessionId.value;
  return result;
}

int _outputBudget(ModelRequest request, _ModelEntry entry) {
  final limit = entry.configuration.descriptor.maxOutputTokens;
  if (limit > 0 && request.maxOutputTokens > limit) {
    throw OpenAIProviderException(
      providerId: entry.provider.id,
      message: 'output budget exceeds model maxTokens',
    );
  }
  return request.maxOutputTokens > 0
      ? request.maxOutputTokens
      : (limit > 0 ? limit.clamp(1, 4096) : 0);
}

List<Object?> _chatMessages(
  List<ModelMessage> messages,
  String systemPrompt,
  ModelOptions options,
) {
  final result = <Object?>[];
  final toolNames = {
    for (final message in messages)
      for (final call in message.toolCalls) call.id: call.name,
  };
  if (systemPrompt.isNotEmpty) {
    result.add(<String, Object?>{
      'role': options.flag('supportsDeveloperRole', false)
          ? 'developer'
          : 'system',
      'content': systemPrompt,
    });
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
      if (options.flag('requiresToolResultName', false)) {
        item['name'] = toolNames[message.toolCallId] ?? 'tool';
      }
    } else {
      item['content'] = message.content.isEmpty
          ? ''
          : _chatContent(message.content);
      if (message.role == ModelMessageRole.assistant &&
          message.continuation?.reasoningSummary.isNotEmpty == true) {
        if (options.flag('requiresThinkingAsText', false)) {
          item['content'] =
              '${message.continuation!.reasoningSummary}\n${item['content']}';
        } else {
          item['reasoning_content'] = message.continuation!.reasoningSummary;
        }
      }
      if (message.role == ModelMessageRole.assistant &&
          options.flag('requiresReasoningContentOnAssistantMessages', false)) {
        item.putIfAbsent('reasoning_content', () => '');
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
    if (options.flag('requiresAssistantAfterToolResult', false) &&
        message.role == ModelMessageRole.user &&
        result.isNotEmpty &&
        (result.last as Map)['role'] == 'tool') {
      result.add({'role': 'assistant', 'content': 'Done.'});
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
  // Continuation replays must not emit function calls that never received an
  // output: interrupted turns can persist the raw provider items without the
  // tool result, and the Responses API rejects unpaired function calls.
  final outputCallIds = <String>{
    for (final message in messages)
      if (message.role == ModelMessageRole.tool && message.toolCallId != null)
        message.toolCallId!.value,
  };
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
        result.addAll(_pairedFunctionCalls(items, outputCallIds));
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
          'content': _responsesContent(message.content, assistant: true),
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
      'content': _responsesContent(
        message.content,
        assistant: message.role == ModelMessageRole.assistant,
      ),
    });
  }
  return result;
}

/// Filters continuation replay items, dropping `function_call` entries whose
/// call never received a `function_call_output`; all other items pass through.
List<Object?> _pairedFunctionCalls(
  List<Object?> items,
  Set<String> outputCallIds,
) => [
  for (final item in items)
    if (item is! Map<String, Object?> ||
        item['type'] != 'function_call' ||
        outputCallIds.contains(item['call_id']))
      item,
];

/// Encodes Responses message content. Assistant messages replayed without a
/// stored continuation are output messages and only accept `output_text`;
/// user and system messages require `input_text`.
List<Object?> _responsesContent(
  List<ContentPart> parts, {
  required bool assistant,
}) {
  final textType = assistant ? 'output_text' : 'input_text';
  return parts.map((part) {
    if (part is TextContent) {
      return <String, Object?>{'type': textType, 'text': part.text};
    }
    if (part is ResourceContent) {
      // The Responses API has no resource block; embed the text as plain text.
      return <String, Object?>{'type': textType, 'text': part.text};
    }
    final image = part as ImageContent;
    return <String, Object?>{
      'type': 'input_image',
      'image_url': image.source,
      'detail': image.detail.name,
    };
  }).toList();
}

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

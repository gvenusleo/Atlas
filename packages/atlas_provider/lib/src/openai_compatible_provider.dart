import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:dio/dio.dart';

import 'sse.dart';

/// The streaming API variant exposed by an OpenAI-compatible endpoint.
enum OpenAIProtocol {
  /// The `/chat/completions` API.
  chatCompletions,

  /// The `/responses` API.
  responses,
}

/// Configuration for one OpenAI-compatible provider endpoint.
final class OpenAIProviderConfiguration {
  /// Creates a provider configuration.
  OpenAIProviderConfiguration({
    required this.id,
    required this.protocol,
    required this.baseUrl,
    required List<OpenAIModelConfiguration> models,
    this.apiKey = '',
    this.userAgent,
  }) : models = List<OpenAIModelConfiguration>.unmodifiable(models);

  /// The provider identifier used by configured model references.
  final ProviderId id;

  /// The API protocol used by this endpoint.
  final OpenAIProtocol protocol;

  /// The endpoint root, without the protocol-specific path.
  final Uri baseUrl;

  /// The bearer token. Empty values omit the authorization header.
  final String apiKey;

  /// An optional user-agent override.
  final String? userAgent;

  /// Models served by this endpoint.
  final List<OpenAIModelConfiguration> models;
}

/// Configuration for one model exposed by an OpenAI-compatible provider.
final class OpenAIModelConfiguration {
  /// Creates a model configuration.
  const OpenAIModelConfiguration({
    required this.descriptor,
    this.promptCacheEnabled = false,
  });

  /// The runtime model descriptor.
  final ModelDescriptor descriptor;

  /// Whether to send the session identifier as a prompt cache key.
  final bool promptCacheEnabled;
}

/// A safe provider failure with the endpoint identity and optional HTTP status.
final class OpenAIProviderException implements Exception {
  /// Creates a provider failure.
  const OpenAIProviderException({
    required this.providerId,
    required this.message,
    this.statusCode,
  });

  /// The provider that reported the failure.
  final ProviderId providerId;

  /// The HTTP status when the server returned one.
  final int? statusCode;

  /// A redacted, provider-safe error message.
  final String message;

  @override
  String toString() {
    final status = statusCode == null ? '' : ' (status $statusCode)';
    return 'OpenAIProviderException[$providerId]$status: $message';
  }
}

/// Streams configured models through the OpenAI Chat Completions or Responses API.
final class OpenAICompatibleProvider implements ModelProvider {
  /// Creates a provider from one or more endpoint configurations.
  OpenAICompatibleProvider(List<OpenAIProviderConfiguration> configurations)
    : _entries = _indexConfigurations(configurations),
      _dio = Dio(
        BaseOptions(
          connectTimeout: const Duration(seconds: 10),
          sendTimeout: const Duration(seconds: 30),
          receiveTimeout: const Duration(minutes: 2),
        ),
      );

  final Map<ModelRef, _ModelEntry> _entries;
  final Dio _dio;

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
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    final entry = _entries[request.model];
    if (entry == null) {
      yield ModelFailedEvent(
        ArgumentError.value(request.model, 'model', 'is not configured'),
        StackTrace.current,
      );
      return;
    }
    _ActiveResponse? active;
    try {
      request.cancellation?.throwIfCancelled();
      _validateRequest(request, entry);
      active = await _openStream(request, entry);
      final parser = entry.provider.protocol == OpenAIProtocol.responses
          ? _ResponsesParser(
              entry.provider.id,
              entry.configuration.descriptor.ref.modelId.value,
              request.maxOutputTokens > 0,
            )
          : _ChatParser(entry.provider.id);
      await for (final event in decodeSse(active.response.data!.stream)) {
        request.cancellation?.throwIfCancelled();
        final updates = parser.accept(event);
        for (final update in updates) {
          yield update;
        }
      }
      request.cancellation?.throwIfCancelled();
      yield ModelCompletedEvent(parser.finish());
    } catch (error, stackTrace) {
      final failure = error is DioException
          ? request.cancellation?.isCancelled == true
                ? const TurnCancelledException()
                : OpenAIProviderException(
                    providerId: entry.provider.id,
                    message: 'stream failed after the response started',
                  )
          : error;
      yield ModelFailedEvent(failure, stackTrace);
    } finally {
      active?.close();
    }
  }

  void _validateRequest(ModelRequest request, _ModelEntry entry) {
    if (request.messages.any(
          (message) => message.content.any((part) => part is ImageContent),
        ) &&
        !entry.configuration.descriptor.inputCapabilities.contains(
          ModelInputCapability.image,
        )) {
      throw OpenAIProviderException(
        providerId: entry.provider.id,
        message: 'model does not support image input',
      );
    }
    if (request.reasoningEffort != null &&
        entry.configuration.descriptor.reasoningEfforts.isNotEmpty &&
        !entry.configuration.descriptor.reasoningEfforts.any(
          (option) => option.value == request.reasoningEffort,
        )) {
      throw OpenAIProviderException(
        providerId: entry.provider.id,
        message: 'unsupported reasoning effort',
      );
    }
  }

  Future<_ActiveResponse> _openStream(
    ModelRequest request,
    _ModelEntry entry,
  ) async {
    final uri = entry.provider.baseUrl.replace(
      path:
          '${entry.provider.baseUrl.path.replaceFirst(RegExp(r'/+$'), '')}${entry.provider.protocol == OpenAIProtocol.responses ? '/responses' : '/chat/completions'}',
    );
    final body = entry.provider.protocol == OpenAIProtocol.responses
        ? _responsesRequest(request, entry)
        : _chatRequest(request, entry);
    final headers = <String, Object>{
      Headers.contentTypeHeader: Headers.jsonContentType,
      Headers.acceptHeader: 'text/event-stream',
      'user-agent': entry.provider.userAgent ?? 'Atlas',
    };
    if (entry.provider.apiKey.isNotEmpty) {
      headers['authorization'] = 'Bearer ${entry.provider.apiKey}';
    }
    Object? lastError;
    for (var attempt = 0; attempt < 4; attempt++) {
      request.cancellation?.throwIfCancelled();
      final cancelToken = CancelToken();
      final cancellation = request.cancellation;
      final requestFinished = Completer<void>();
      final cancelSubscription = cancellation == null
          ? null
          : Future.any<void>([
              cancellation.whenCancelled,
              requestFinished.future,
            ]).then((_) {
              if (!requestFinished.isCompleted && !cancelToken.isCancelled) {
                cancelToken.cancel();
              }
            });
      var keepCancellationActive = false;
      try {
        final response = await _dio.postUri<ResponseBody>(
          uri,
          data: jsonEncode(body),
          cancelToken: cancelToken,
          options: Options(
            responseType: ResponseType.stream,
            headers: headers,
            validateStatus: (_) => true,
          ),
        );
        final status = response.statusCode ?? 0;
        if (status >= 200 && status < 300) {
          keepCancellationActive = true;
          return _ActiveResponse(
            response: response,
            finished: requestFinished,
            cancellationListener: cancelSubscription,
          );
        }
        final retryable = status == 429 || status >= 500;
        lastError = OpenAIProviderException(
          providerId: entry.provider.id,
          statusCode: status,
          message: await _errorBody(
            response.data?.stream,
            entry.provider.apiKey,
          ),
        );
        if (!retryable || attempt == 3) {
          throw lastError;
        }
        await _waitBeforeRetry(
          request.cancellation,
          _retryDelay(attempt, response.headers),
        );
      } on DioException catch (_) {
        if (cancelToken.isCancelled || cancellation?.isCancelled == true) {
          throw const TurnCancelledException();
        }
        lastError = OpenAIProviderException(
          providerId: entry.provider.id,
          message: 'request failed before streaming started',
        );
        if (attempt == 3) {
          throw lastError;
        }
        await _waitBeforeRetry(
          request.cancellation,
          _retryDelay(attempt, null),
        );
      } finally {
        if (!keepCancellationActive) {
          requestFinished.complete();
          unawaited(cancelSubscription);
        }
      }
    }
    throw lastError ?? StateError('request attempts exhausted');
  }
}

final class _ModelEntry {
  const _ModelEntry(this.provider, this.configuration);

  final OpenAIProviderConfiguration provider;
  final OpenAIModelConfiguration configuration;
}

final class _ActiveResponse {
  const _ActiveResponse({
    required this.response,
    required this.finished,
    required this.cancellationListener,
  });

  final Response<ResponseBody> response;
  final Completer<void> finished;
  final Future<void>? cancellationListener;

  void close() {
    if (!finished.isCompleted) {
      finished.complete();
    }
    unawaited(cancellationListener);
  }
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
  final tools = _tools(request.tools, responses: false);
  if (tools.isNotEmpty) {
    result['tools'] = tools;
  }
  if (request.maxOutputTokens > 0) {
    result['max_tokens'] = request.maxOutputTokens;
  }
  if (request.temperature != null) {
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
  if (request.temperature != null) result['temperature'] = request.temperature;
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

abstract interface class _Parser {
  Iterable<ModelStreamEvent> accept(SseEvent event);

  ModelResponse finish();
}

final class _ChatParser implements _Parser {
  _ChatParser(this.providerId);

  final ProviderId providerId;
  final _reasoning = StringBuffer();
  final _content = StringBuffer();
  final _tools = <int, _ToolAccumulator>{};
  TokenUsage _usage = const TokenUsage();
  String? _finishReason;
  bool _sawEvent = false;
  bool _done = false;

  @override
  Iterable<ModelStreamEvent> accept(SseEvent event) sync* {
    _sawEvent = true;
    if (event.data == '[DONE]') {
      _done = true;
      return;
    }
    final value = _asMap(jsonDecode(event.data));
    final usage = _asMap(value['usage']);
    if (usage.isNotEmpty) _usage = _chatUsage(usage);
    final choices = value['choices'];
    if (choices is! List || choices.isEmpty) return;
    final choice = _asMap(choices.first);
    final delta = _asMap(choice['delta']);
    final text = delta['content'];
    if (text is String && text.isNotEmpty) {
      _content.write(text);
      yield TextDeltaEvent(text);
    }
    final reasoning = delta['reasoning_content'];
    if (reasoning is String && reasoning.isNotEmpty) {
      _reasoning.write(reasoning);
      yield ReasoningDeltaEvent(reasoning);
    }
    final calls = delta['tool_calls'];
    if (calls is List) {
      for (final raw in calls) {
        final call = _asMap(raw);
        final index = call['index'] as int? ?? 0;
        final function = _asMap(call['function']);
        final accumulator = _tools.putIfAbsent(index, _ToolAccumulator.new);
        accumulator.id = (call['id'] as String?) ?? accumulator.id;
        accumulator.name = (function['name'] as String?) ?? accumulator.name;
        accumulator.arguments.write(function['arguments'] as String? ?? '');
      }
    }
    final finish = choice['finish_reason'];
    if (finish is String && finish.isNotEmpty) _finishReason = finish;
  }

  @override
  ModelResponse finish() {
    if (!_sawEvent || !_done || _finishReason == null) {
      throw OpenAIProviderException(
        providerId: providerId,
        message: 'chat stream ended without completion',
      );
    }
    final calls = <ToolCall>[];
    for (final entry in _tools.entries) {
      final call = entry.value;
      final id = call.id;
      final name = call.name;
      if (id == null || name == null) {
        throw OpenAIProviderException(
          providerId: providerId,
          message: 'chat stream returned an incomplete tool call',
        );
      }
      final decoded = jsonDecode(call.arguments.toString());
      if (decoded is! Map<String, Object?>) {
        throw OpenAIProviderException(
          providerId: providerId,
          message: 'chat stream returned invalid tool arguments',
        );
      }
      calls.add(
        ToolCall(
          id: ToolCallId(id),
          name: name,
          arguments: _freezeObject(decoded),
        ),
      );
    }
    final reason = calls.isNotEmpty
        ? StopReason.toolUse
        : switch (_finishReason) {
            'stop' => StopReason.endTurn,
            'length' => StopReason.maxTokens,
            _ => StopReason.unknown,
          };
    return ModelResponse(
      content: _content.isEmpty ? const [] : [TextContent(_content.toString())],
      toolCalls: calls,
      stopReason: reason,
      usage: _usage,
      continuation: _reasoning.isEmpty
          ? null
          : ModelContinuation(
              providerId: providerId,
              reasoningSummary: _reasoning.toString(),
            ),
    );
  }
}

final class _ResponsesParser implements _Parser {
  _ResponsesParser(this.providerId, this.modelId, this.hasOutputLimit);

  final ProviderId providerId;
  final String modelId;
  final bool hasOutputLimit;
  final _reasoning = StringBuffer();
  final _content = StringBuffer();
  final _fallbackContent = StringBuffer();
  final _calls = <ToolCall>[];
  final _items = <Object?>[];
  TokenUsage _usage = const TokenUsage();
  String? _status;
  String? _incompleteReason;
  String? _failure;
  bool _sawEvent = false;

  @override
  Iterable<ModelStreamEvent> accept(SseEvent event) sync* {
    _sawEvent = true;
    if (event.data == '[DONE]') return;
    final value = _asMap(jsonDecode(event.data));
    final type = value['type'] as String? ?? event.name;
    final delta = value['delta'];
    if (type == 'response.output_text.delta' && delta is String) {
      _content.write(delta);
      yield TextDeltaEvent(delta);
    } else if ((type == 'response.reasoning_summary_text.delta' ||
            type == 'response.reasoning.delta') &&
        delta is String) {
      _reasoning.write(delta);
      yield ReasoningDeltaEvent(delta);
    } else if (type == 'response.output_item.done') {
      _captureOutput(_asMap(value['item']));
    } else if (type == 'response.completed' || type == 'response.incomplete') {
      final response = _asMap(value['response']);
      _status =
          response['status'] as String? ??
          (type == 'response.completed' ? 'completed' : 'incomplete');
      _incompleteReason =
          _asMap(response['incomplete_details'])['reason'] as String?;
      _usage = _responsesUsage(_asMap(response['usage']));
      final output = response['output'];
      if (output is List) {
        for (final item in output) {
          _captureOutput(_asMap(item));
        }
      }
    } else if (type == 'response.failed') {
      _failure =
          _asMap(value['error'])['message'] as String? ??
          'responses request failed';
      _status = 'failed';
    }
  }

  @override
  ModelResponse finish() {
    if (!_sawEvent || _status == null) {
      throw OpenAIProviderException(
        providerId: providerId,
        message: 'responses stream ended without completion',
      );
    }
    if (_failure != null || _status == 'failed') {
      throw OpenAIProviderException(
        providerId: providerId,
        message: _failure ?? 'responses request failed',
      );
    }
    final reason = _calls.isNotEmpty
        ? StopReason.toolUse
        : _status == 'completed'
        ? StopReason.endTurn
        : _status == 'incomplete'
        ? _incompleteReason == 'max_output_tokens' ||
                  (_incompleteReason == null && hasOutputLimit)
              ? StopReason.maxTokens
              : StopReason.unknown
        : StopReason.unknown;
    return ModelResponse(
      content: (_content.isEmpty ? _fallbackContent : _content).isEmpty
          ? const []
          : [
              TextContent(
                (_content.isEmpty ? _fallbackContent : _content).toString(),
              ),
            ],
      toolCalls: List.unmodifiable(_calls),
      stopReason: reason,
      usage: _usage,
      continuation: ModelContinuation(
        providerId: providerId,
        reasoningSummary: _reasoning.toString(),
        opaquePayload: <String, Object?>{
          'protocol': 'responses',
          'model': modelId,
          'items': List<Object?>.unmodifiable(_items),
        },
      ),
    );
  }

  void _captureOutput(Map<String, Object?> item) {
    final frozen = freezeJson(item);
    if (!_items.any((existing) => jsonEncode(existing) == jsonEncode(frozen))) {
      _items.add(frozen);
    }
    if (item['type'] == 'message') {
      final content = item['content'];
      if (content is List) {
        for (final part in content) {
          final value = _asMap(part);
          final text = value['text'];
          if (text is String && value['type'] != 'reasoning') {
            _fallbackContent.write(text);
          }
        }
      }
    }
    if (item['type'] != 'function_call' ||
        _calls.any((call) => call.id.value == item['call_id'])) {
      return;
    }
    final id = item['call_id'] as String?;
    final name = item['name'] as String?;
    final arguments = item['arguments'] as String?;
    if (id == null || name == null || arguments == null) {
      throw OpenAIProviderException(
        providerId: providerId,
        message: 'responses stream returned an incomplete tool call',
      );
    }
    final decoded = jsonDecode(arguments);
    if (decoded is! Map<String, Object?>) {
      throw OpenAIProviderException(
        providerId: providerId,
        message: 'responses stream returned invalid tool arguments',
      );
    }
    _calls.add(
      ToolCall(
        id: ToolCallId(id),
        name: name,
        arguments: _freezeObject(decoded),
      ),
    );
  }
}

Map<String, Object?> _asMap(Object? value) => value is Map
    ? value.map((key, value) => MapEntry(key.toString(), value))
    : <String, Object?>{};

JsonObject _freezeObject(Map<String, Object?> value) =>
    Map<String, Object?>.unmodifiable(
      value.map((key, nested) => MapEntry(key, freezeJson(nested))),
    );

TokenUsage _chatUsage(Map<String, Object?> value) => TokenUsage(
  inputTokens: _int(value['prompt_tokens']),
  outputTokens: _int(value['completion_tokens']),
  totalTokens: _int(value['total_tokens']),
  cacheReadInputTokens: _int(value['cache_read_input_tokens']) > 0
      ? _int(value['cache_read_input_tokens'])
      : _int(_asMap(value['prompt_tokens_details'])['cached_tokens']),
  cacheWriteInputTokens: _int(value['cache_write_input_tokens']) > 0
      ? _int(value['cache_write_input_tokens'])
      : _int(
          _asMap(value['prompt_tokens_details'])['cache_creation_input_tokens'],
        ),
);

TokenUsage _responsesUsage(Map<String, Object?> value) => TokenUsage(
  inputTokens: _int(value['input_tokens']),
  outputTokens: _int(value['output_tokens']),
  totalTokens: _int(value['total_tokens']),
  cacheReadInputTokens: _int(
    _asMap(value['input_tokens_details'])['cached_tokens'],
  ),
);

int _int(Object? value) => value is num ? value.toInt() : 0;

Future<String> _errorBody(Stream<List<int>>? stream, String secret) async {
  if (stream == null) return 'provider returned an error';
  final bytes = <int>[];
  await for (final chunk in stream) {
    final remaining = 65536 - bytes.length;
    if (remaining > 0) {
      bytes.addAll(chunk.take(remaining));
    }
  }
  final text = utf8.decode(bytes, allowMalformed: true).trim();
  if (text.isEmpty) return 'provider returned an error';
  return secret.isEmpty ? text : text.replaceAll(secret, '[redacted]');
}

Duration _retryDelay(int attempt, Headers? headers) {
  final retryAfter = headers?.value('retry-after');
  final seconds = num.tryParse(retryAfter ?? '');
  if (seconds != null && seconds >= 0) {
    return Duration(milliseconds: (seconds * 1000).round());
  }
  return Duration(seconds: math.pow(2, attempt).toInt());
}

Future<void> _waitBeforeRetry(CancellationToken? token, Duration delay) async {
  if (token == null) {
    await Future<void>.delayed(delay);
    return;
  }
  await Future.any<void>([Future<void>.delayed(delay), token.whenCancelled]);
  token.throwIfCancelled();
}

final class _ToolAccumulator {
  String? id;
  String? name;
  final arguments = StringBuffer();
}

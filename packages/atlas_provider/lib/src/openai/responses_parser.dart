import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';

import '../json_utils.dart';
import '../sse.dart';
import '../stream_runner.dart';
import 'openai_configuration.dart';

/// Parses an OpenAI Responses streaming response.
final class ResponsesParser(
  /// The provider that owns this stream.
  final ProviderId providerId,

  /// The provider-local model identifier.
  final String modelId,

  /// Whether the request carried an explicit output token limit.
  final bool hasOutputLimit,
) implements StreamParser {
  /// Creates a responses parser for [providerId] and [modelId].
  this;

  final _reasoning = StringBuffer();
  final _content = StringBuffer();
  final _fallbackContent = StringBuffer();
  final _calls = <ToolCall>[];
  final _items = <Object?>[];
  final _itemKeys = <String>{};
  TokenUsage _usage = const TokenUsage();
  String? _status;
  String? _incompleteReason;
  String? _failure;
  String? _failureDetail;
  bool _sawEvent = false;

  @override
  Iterable<ModelStreamEvent> accept(SseEvent event) sync* {
    _sawEvent = true;
    if (event.data == '[DONE]') return;
    final value = asJsonMap(jsonDecode(event.data));
    final type = value['type'] as String? ?? event.name;
    switch ((type, value)) {
      case ('response.output_text.delta', {'delta': String delta}):
        _content.write(delta);
        yield TextDeltaEvent(delta);
      case (
        'response.reasoning_summary_text.delta' || 'response.reasoning.delta',
        {'delta': String delta},
      ):
        _reasoning.write(delta);
        yield ReasoningDeltaEvent(delta);
      case ('response.output_item.done', _):
        _captureOutput(asJsonMap(value['item']));
      case ('response.completed' || 'response.incomplete', _):
        final response = asJsonMap(value['response']);
        _status =
            response['status'] as String? ??
            (type == 'response.completed' ? 'completed' : 'incomplete');
        _incompleteReason =
            asJsonMap(response['incomplete_details'])['reason'] as String?;
        _usage = _responsesUsage(asJsonMap(response['usage']));
        final output = response['output'];
        if (output is List) {
          for (final item in output) {
            _captureOutput(asJsonMap(item));
          }
        }
      case ('response.failed' || 'error', _):
        _failure = 'responses request failed';
        _failureDetail = _errorMessage(value);
        _status = 'failed';
    }
  }

  static String? _errorMessage(Map<String, Object?> value) {
    final error = asJsonMap(value['error']);
    final message = error['message'] ?? value['message'];
    return message is String && message.trim().isNotEmpty ? message : null;
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
        detail: _failureDetail,
      );
    }
    final reason = _calls.isNotEmpty
        ? StopReason.toolUse
        : switch ((_status, _incompleteReason)) {
            ('completed', _) => StopReason.endTurn,
            ('incomplete', 'max_output_tokens') => StopReason.maxTokens,
            ('incomplete', null) when hasOutputLimit => StopReason.maxTokens,
            _ => StopReason.unknown,
          };
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
      reasoning: _reasoning.toString(),
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
    if (_itemKeys.add(jsonEncode(frozen))) {
      _items.add(frozen);
    }
    if (item['type'] == 'message' && _fallbackContent.isEmpty) {
      final content = item['content'];
      if (content is List) {
        for (final part in content) {
          final value = asJsonMap(part);
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
    if (id == null ||
        id.isEmpty ||
        name == null ||
        name.isEmpty ||
        arguments == null) {
      throw OpenAIProviderException(
        providerId: providerId,
        message: 'responses stream returned an incomplete tool call',
      );
    }
    final Object? decoded;
    try {
      decoded = arguments.isEmpty
          ? const <String, Object?>{}
          : jsonDecode(arguments);
    } on FormatException {
      throw OpenAIProviderException(
        providerId: providerId,
        message: 'responses stream returned invalid tool arguments',
      );
    }
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
        arguments: immutableJsonObject(decoded),
      ),
    );
  }
}

TokenUsage _responsesUsage(Map<String, Object?> value) {
  final details = asJsonMap(value['input_tokens_details']);
  final read = tokenCount(details['cached_tokens']);
  final write = tokenCount(details['cache_write_tokens']);
  return TokenUsage(
    inputTokens: asInt(value['input_tokens']),
    outputTokens: asInt(value['output_tokens']),
    totalTokens: asInt(value['total_tokens']),
    cacheReadInputTokens: read ?? 0,
    cacheWriteInputTokens: write ?? 0,
    promptTokens: tokenCount(value['input_tokens']),
    cacheReadReported: read != null,
    cacheWriteReported: write != null,
  );
}

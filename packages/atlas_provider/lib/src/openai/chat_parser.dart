import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';

import '../json_utils.dart';
import '../sse.dart';
import '../stream_runner.dart';
import 'openai_configuration.dart';

/// Parses a Chat Completions streaming response.
final class ChatParser implements StreamParser {
  /// Creates a chat parser for [providerId].
  ChatParser(this.providerId);

  /// The provider that owns this stream.
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
    final value = asJsonMap(jsonDecode(event.data));
    final usage = asJsonMap(value['usage']);
    if (usage.isNotEmpty) _usage = _chatUsage(usage);
    final choices = value['choices'];
    if (choices is! List || choices.isEmpty) return;
    final choice = asJsonMap(choices.first);
    final delta = asJsonMap(choice['delta']);
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
        final call = asJsonMap(raw);
        final index = call['index'] as int? ?? 0;
        final function = asJsonMap(call['function']);
        final accumulator = _tools.putIfAbsent(index, _ToolAccumulator.new);
        final id = call['id'];
        if (id is String && id.isNotEmpty) accumulator.id = id;
        final name = function['name'];
        if (name is String && name.isNotEmpty) accumulator.name = name;
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
          arguments: immutableJsonObject(decoded),
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

TokenUsage _chatUsage(Map<String, Object?> value) => TokenUsage(
  inputTokens: asInt(value['prompt_tokens']),
  outputTokens: asInt(value['completion_tokens']),
  totalTokens: asInt(value['total_tokens']),
  cacheReadInputTokens: asInt(value['cache_read_input_tokens']) > 0
      ? asInt(value['cache_read_input_tokens'])
      : asInt(asJsonMap(value['prompt_tokens_details'])['cached_tokens']),
  cacheWriteInputTokens: asInt(value['cache_write_input_tokens']) > 0
      ? asInt(value['cache_write_input_tokens'])
      : asInt(
          asJsonMap(
            value['prompt_tokens_details'],
          )['cache_creation_input_tokens'],
        ),
);

final class _ToolAccumulator {
  String? id;
  String? name;
  final arguments = StringBuffer();
}

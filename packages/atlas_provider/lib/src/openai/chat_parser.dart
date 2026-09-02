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
  _ToolAccumulator? _lastTool;
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
    // DeepSeek natively streams reasoning under `reasoning_content`; some
    // relays (e.g. CommandCode) rewrite it to `reasoning`, so accept both.
    final reasoning = delta['reasoning_content'] ?? delta['reasoning'];
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
        final id = call['id'];
        final name = function['name'];
        var accumulator = _tools[index];
        if (accumulator == null) {
          // Some relays number id/name chunks and arguments chunks with
          // unrelated indexes, so an arguments-only chunk may not match
          // the slot of its call. Pair it with the most recently started
          // call instead of opening a new slot.
          final startsCall =
              id is String && id.isNotEmpty ||
              name is String && name.isNotEmpty;
          if (startsCall || _lastTool == null) {
            accumulator = _ToolAccumulator();
            _tools[index] = accumulator;
          } else {
            accumulator = _lastTool!;
          }
        }
        if (id is String && id.isNotEmpty) accumulator.id = id;
        if (name is String && name.isNotEmpty) accumulator.name = name;
        accumulator.arguments.write(function['arguments'] as String? ?? '');
        _lastTool = accumulator;
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
          message:
              'chat stream returned an incomplete tool call: '
              'index ${entry.key} is missing '
              '${[if (id == null) 'id', if (name == null) 'name'].join(' and ')}'
              ' (${call.arguments.length} argument characters received)',
        );
      }
      final argumentsText = call.arguments.toString();
      final Object? decoded;
      try {
        decoded = argumentsText.isEmpty
            ? const <String, Object?>{}
            : jsonDecode(argumentsText);
      } on FormatException {
        throw OpenAIProviderException(
          providerId: providerId,
          message: 'chat stream returned invalid tool arguments',
        );
      }
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
      reasoning: _reasoning.toString(),
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

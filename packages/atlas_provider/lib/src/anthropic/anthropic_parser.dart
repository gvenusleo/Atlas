import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';

import '../json_utils.dart';
import '../sse.dart';
import '../stream_runner.dart';
import 'anthropic_configuration.dart';

/// Parses an Anthropic Messages streaming response.
final class AnthropicParser implements StreamParser {
  /// Creates a parser for [providerId].
  AnthropicParser(this.providerId);

  /// The provider that owns this stream.
  final ProviderId providerId;
  final _thinkingBlocks = <Map<String, Object?>>[];
  _ThinkingBlock? _currentThinking;
  final _reasoning = StringBuffer();
  final _content = StringBuffer();
  final _toolBlocks = <int, _ToolBlock>{};
  int _inputTokens = 0;
  int _cacheReadTokens = 0;
  int _cacheWriteTokens = 0;
  int _outputTokens = 0;
  String? _stopReason;
  String? _failure;
  bool _sawMessageStop = false;
  bool _sawEvent = false;

  /// Accepts one SSE event and yields incremental model events.
  @override
  Iterable<ModelStreamEvent> accept(SseEvent event) sync* {
    _sawEvent = true;
    final value = asJsonMap(jsonDecode(event.data));
    final type = value['type'] as String? ?? event.name;
    switch (type) {
      case 'message_start':
        final message = asJsonMap(value['message']);
        final usage = asJsonMap(message['usage']);
        _inputTokens = asInt(usage['input_tokens']);
        _cacheReadTokens = asInt(usage['cache_read_input_tokens']);
        _cacheWriteTokens = asInt(usage['cache_creation_input_tokens']);
      case 'content_block_start':
        final block = asJsonMap(value['content_block']);
        switch (block['type']) {
          case 'tool_use':
            _toolBlocks.putIfAbsent(
                value['index'] as int? ?? _toolBlocks.length,
                _ToolBlock.new,
              )
              ..id = block['id'] as String?
              ..name = block['name'] as String?;
          case 'thinking':
            _currentThinking = _ThinkingBlock(
              signature: block['signature'] as String? ?? '',
            );
          case 'redacted_thinking':
            final data = block['data'];
            if (data is String && data.isNotEmpty) {
              _thinkingBlocks.add(<String, Object?>{
                'type': 'redacted_thinking',
                'data': data,
              });
            }
        }
      case 'content_block_delta':
        final delta = asJsonMap(value['delta']);
        final index = value['index'] as int?;
        switch (delta['type']) {
          case 'text_delta':
            final text = delta['text'];
            if (text is String && text.isNotEmpty) {
              _content.write(text);
              yield TextDeltaEvent(text);
            }
          case 'thinking_delta':
            final thinking = delta['thinking'];
            if (thinking is String && thinking.isNotEmpty) {
              _currentThinking?.text.write(thinking);
              _reasoning.write(thinking);
              yield ReasoningDeltaEvent(thinking);
            }
          case 'signature_delta':
            final signature = delta['signature'];
            if (signature is String) {
              _currentThinking?.signature = signature;
            }
          case 'input_json_delta':
            final partial = delta['partial_json'];
            if (partial is String) {
              _toolBlocks
                  .putIfAbsent(index ?? _toolBlocks.length, _ToolBlock.new)
                  .input
                  .write(partial);
            }
        }
      case 'content_block_stop':
        final thinkingBlock = _currentThinking;
        if (thinkingBlock != null) {
          if (thinkingBlock.signature.isNotEmpty) {
            _thinkingBlocks.add(<String, Object?>{
              'type': 'thinking',
              'thinking': thinkingBlock.text.toString(),
              'signature': thinkingBlock.signature,
            });
          }
          _currentThinking = null;
        }
        final index = value['index'] as int?;
        if (index != null) {
          final block = _toolBlocks[index];
          if (block != null) {
            block.completeInput();
          }
        }
      case 'message_delta':
        final delta = asJsonMap(value['delta']);
        _stopReason = delta['stop_reason'] as String? ?? _stopReason;
        _outputTokens = asInt(asJsonMap(value['usage'])['output_tokens']);
      case 'message_stop':
        _sawMessageStop = true;
      case 'error':
        _failure = 'anthropic stream failed';
      case 'ping':
        break;
    }
  }

  /// Returns the accumulated response once the stream is complete.
  @override
  ModelResponse finish() {
    if (!_sawEvent || (!_sawMessageStop && _failure == null)) {
      throw AnthropicProviderException(
        providerId: providerId,
        message: 'anthropic stream ended without completion',
      );
    }
    if (_failure != null) {
      throw AnthropicProviderException(
        providerId: providerId,
        message: _failure!,
      );
    }
    final calls = <ToolCall>[];
    for (final block in _toolBlocks.entries) {
      final tool = block.value;
      final id = tool.id;
      final name = tool.name;
      if (id == null || name == null) {
        throw AnthropicProviderException(
          providerId: providerId,
          message: 'anthropic stream returned an incomplete tool use',
        );
      }
      final decoded = tool.inputJson;
      if (decoded == null) {
        throw AnthropicProviderException(
          providerId: providerId,
          message: 'anthropic stream returned invalid tool input',
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
        : switch (_stopReason) {
            'end_turn' => StopReason.endTurn,
            'max_tokens' => StopReason.maxTokens,
            _ => StopReason.unknown,
          };
    return ModelResponse(
      content: _content.isEmpty ? const [] : [TextContent(_content.toString())],
      toolCalls: calls,
      stopReason: reason,
      usage: TokenUsage(
        inputTokens: _inputTokens,
        outputTokens: _outputTokens,
        totalTokens: _inputTokens + _outputTokens,
        cacheReadInputTokens: _cacheReadTokens,
        cacheWriteInputTokens: _cacheWriteTokens,
      ),
      continuation: _thinkingBlocks.isEmpty
          ? null
          : ModelContinuation(
              providerId: providerId,
              reasoningSummary: _reasoning.toString(),
              opaquePayload: <String, Object?>{
                'thinking_blocks': List<Map<String, Object?>>.unmodifiable(
                  _thinkingBlocks,
                ),
              },
            ),
    );
  }
}

final class _ThinkingBlock {
  _ThinkingBlock({required this.signature});

  String signature;
  final text = StringBuffer();
}

final class _ToolBlock {
  String? id;
  String? name;
  final input = StringBuffer();
  Map<String, Object?>? _parsedInput;

  void completeInput() {
    if (_parsedInput != null || input.isEmpty) {
      return;
    }
    try {
      final decoded = jsonDecode(input.toString());
      if (decoded is Map<String, Object?>) {
        _parsedInput = decoded;
      }
    } on FormatException {
      // Keep null; finish() reports the invalid tool input.
    }
  }

  Map<String, Object?>? get inputJson => _parsedInput;
}

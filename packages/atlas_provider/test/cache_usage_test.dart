import 'dart:convert';

import 'package:atlas_provider/src/anthropic/anthropic_parser.dart';
import 'package:atlas_provider/src/openai/chat_parser.dart';
import 'package:atlas_provider/src/openai/responses_parser.dart';
import 'package:atlas_provider/src/sse.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  for (final chat in [false, true]) {
    final protocol = chat ? 'Chat' : 'Responses';
    final inputKey = chat ? 'prompt_tokens' : 'input_tokens';
    final detailsKey = chat ? 'prompt_tokens_details' : 'input_tokens_details';
    TokenUsage parse(Map<String, Object?> usage) =>
        chat ? _chat(usage) : _responses(usage);

    test('$protocol normalizes subsets and preserves cache writes', () {
      final usage = parse({
        inputKey: 15000,
        detailsKey: {'cached_tokens': 12000, 'cache_write_tokens': 3000},
      });
      expect(usage.inputTokens, 15000);
      expect(usage.promptTokens, 15000);
      expect(usage.cacheReadInputTokens, 12000);
      expect(usage.cacheWriteInputTokens, 3000);
      expect(usage.cacheReadReported, isTrue);
      expect(usage.cacheWriteReported, isTrue);
    });

    test('$protocol distinguishes absent fields from explicit zero', () {
      final absent = parse({inputKey: 1000});
      expect(absent.promptTokens, 1000);
      expect(absent.cacheReadReported, isFalse);
      expect(absent.cacheWriteReported, isFalse);
      final zero = parse({
        inputKey: 1000,
        detailsKey: {'cached_tokens': 0, 'cache_write_tokens': 0},
      });
      expect(zero.cacheReadReported, isTrue);
      expect(zero.cacheWriteReported, isTrue);
      expect(zero.cacheReadInputTokens, 0);
      expect(zero.cacheWriteInputTokens, 0);
    });

    test('$protocol rejects malformed counts as unknown', () {
      for (final value in [null, -1, 1.5, '12']) {
        final usage = parse({
          inputKey: value,
          detailsKey: {'cached_tokens': value, 'cache_write_tokens': value},
        });
        expect(usage.promptTokens, isNull);
        expect(usage.cacheReadReported, isFalse);
        expect(usage.cacheWriteReported, isFalse);
      }
    });
  }

  test('Chat custom top-level buckets cannot establish input convention', () {
    for (final cached in [400, 1400]) {
      final usage = _chat({
        'prompt_tokens': 1000,
        'cache_read_input_tokens': cached,
        'cache_write_input_tokens': 50,
      });
      expect(usage.promptTokens, isNull);
      expect(usage.cacheReadInputTokens, cached);
      expect(usage.cacheWriteInputTokens, 50);
      expect(usage.cacheReadReported, isTrue);
    }
  });

  test(
    'Chat standard cached_tokens zero wins over conflicting custom fields',
    () {
      final usage = _chat({
        'prompt_tokens': 1000,
        'cache_read_input_tokens': 900,
        'prompt_tokens_details': {
          'cached_tokens': 0,
          'cache_creation_input_tokens': 100,
        },
      });
      expect(usage.promptTokens, 1000);
      expect(usage.cacheReadInputTokens, 0);
      expect(usage.cacheWriteInputTokens, 100);
    },
  );

  test('Anthropic adds separate cache buckets once', () {
    final usage = _anthropic({
      'input_tokens': 1000,
      'cache_read_input_tokens': 400,
      'cache_creation_input_tokens': 50,
    });
    expect(usage.inputTokens, 1000);
    expect(usage.promptTokens, 1450);
    expect(usage.cacheReadInputTokens, 400);
    expect(usage.cacheWriteInputTokens, 50);
    expect(usage.cacheReadReported, isTrue);
    expect(usage.cacheWriteReported, isTrue);
  });

  test('Anthropic missing or invalid buckets leave denominator unknown', () {
    for (final fields in [
      <String, Object?>{'input_tokens': 1000},
      <String, Object?>{'input_tokens': 1000, 'cache_read_input_tokens': 0},
      <String, Object?>{
        'input_tokens': 1000,
        'cache_read_input_tokens': 0,
        'cache_creation_input_tokens': -1,
      },
    ]) {
      expect(_anthropic(fields).promptTokens, isNull);
    }
    final explicit = _anthropic({
      'input_tokens': 1000,
      'cache_read_input_tokens': 0,
      'cache_creation_input_tokens': 0,
    });
    expect(explicit.promptTokens, 1000);
    expect(explicit.cacheReadReported, isTrue);
    expect(explicit.cacheWriteReported, isTrue);
  });
}

TokenUsage _responses(Map<String, Object?> usage) {
  final parser = ResponsesParser(ProviderId('test'), 'model', false);
  parser
      .accept(
        SseEvent(
          null,
          jsonEncode({
            'type': 'response.completed',
            'response': {'status': 'completed', 'usage': usage},
          }),
        ),
      )
      .toList();
  return parser.finish().usage;
}

TokenUsage _chat(Map<String, Object?> usage) {
  final parser = ChatParser(ProviderId('test'));
  parser
      .accept(
        SseEvent(
          null,
          jsonEncode({
            'usage': usage,
            'choices': [
              {'delta': <String, Object?>{}, 'finish_reason': 'stop'},
            ],
          }),
        ),
      )
      .toList();
  parser.accept(const SseEvent(null, '[DONE]')).toList();
  return parser.finish().usage;
}

TokenUsage _anthropic(Map<String, Object?> usage) {
  final parser = AnthropicParser(ProviderId('test'));
  parser
      .accept(
        SseEvent(
          null,
          jsonEncode({
            'type': 'message_start',
            'message': {'usage': usage},
          }),
        ),
      )
      .toList();
  parser.accept(const SseEvent(null, '{"type":"message_stop"}')).toList();
  return parser.finish().usage;
}

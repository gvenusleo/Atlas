import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/src/mappers/timeline_codec.dart';
import 'package:test/test.dart';

void main() {
  final codec = TimelineCodec();
  final item = AssistantMessageItem(
    id: TimelineItemId('assistant'),
    sessionId: SessionId('session'),
    turnId: TurnId('turn'),
    sequence: 1,
    occurredAt: DateTime.utc(2026),
    content: const [TextContent('response')],
    model: ModelRef(
      providerId: ProviderId('retired'),
      modelId: ModelId('model'),
    ),
    stopReason: StopReason.endTurn,
    usage: const TokenUsage(
      inputTokens: 100,
      outputTokens: 10,
      cacheReadInputTokens: 900,
      cacheWriteInputTokens: 50,
      promptTokens: 1050,
      cacheReadReported: true,
      cacheWriteReported: true,
    ),
  );

  AssistantMessageItem decode(String payload) =>
      codec
              .decode(
                id: item.id,
                sessionId: item.sessionId,
                turnId: item.turnId,
                sequence: item.sequence,
                occurredAt: item.occurredAt,
                kind: 'assistant_message',
                version: 1,
                payload: payload,
              )
              .item
          as AssistantMessageItem;

  test('round trips normalized accounting alongside original usage', () {
    final result = decode(codec.encode(item).payload).usage;
    expect(result.inputTokens, 100);
    expect(result.promptTokens, 1050);
    expect(result.cacheReadInputTokens, 900);
    expect(result.cacheWriteInputTokens, 50);
    expect(result.cacheReadReported, isTrue);
    expect(result.cacheWriteReported, isTrue);
  });

  test('legacy payload remains readable without inventing cache metadata', () {
    final payload =
        jsonDecode(codec.encode(item).payload) as Map<String, dynamic>;
    final usage = payload['usage'] as Map<String, dynamic>;
    usage.remove('prompt_tokens');
    usage.remove('cache_read_reported');
    usage.remove('cache_write_reported');
    final result = decode(jsonEncode(payload)).usage;
    expect(result.inputTokens, 100);
    expect(result.cacheReadInputTokens, 900);
    expect(result.cacheWriteInputTokens, 50);
    expect(result.promptTokens, isNull);
    expect(result.cacheReadReported, isFalse);
    expect(result.cacheWriteReported, isFalse);
  });
}

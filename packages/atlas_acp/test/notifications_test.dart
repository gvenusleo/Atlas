import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'acp_support.dart';

void main() {
  test('emits session_info_update when a prompt generates a title', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Inspect the files'},
        ],
      },
    });
    final updates = await wire.turnNotifications.take(6).toList();
    final info =
        ((updates[5]['params'] as Map)['update'] as Map<String, Object?>);
    expect(info['sessionUpdate'], 'session_info_update');
    expect(info['title'], 'Inspect the files');
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('emits usage_update with used tokens and context size', () async {
    final wire = await Wire.open(
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
          usage: TokenUsage(totalTokens: 1200),
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    final usageUpdates = wire.turnNotifications.where((message) {
      final update = (message['params'] as Map?)?['update'];
      return update is Map && update['sessionUpdate'] == 'usage_update';
    });
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Summarize'},
        ],
      },
    });
    // The mid-turn report from the model response and the turn-end report
    // carry the same figure, so exactly one update reaches the client.
    final usageMessage = await usageUpdates.first;
    await promptFuture;
    final usage =
        ((usageMessage['params'] as Map)['update'] as Map<String, Object?>);
    expect(usage['used'], 1200);
    expect(usage['size'], 128000);
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('emits usage_update after each intermediate model response', () async {
    final wire = await Wire.open(
      responses: [
        ModelResponse(
          content: const [TextContent('I will inspect the files.')],
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'read',
              arguments: <String, Object?>{'path': '.'},
            ),
          ],
          stopReason: StopReason.toolUse,
          usage: const TokenUsage(totalTokens: 900),
        ),
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Inspect the files'},
        ],
      },
    });
    // The scripted default turn ends with an empty usage, so the only
    // usage_update can come from the intermediate model response.
    final updates = await wire.turnNotifications
        .where(
          (message) =>
              ((message['params'] as Map?)?['update']
                  as Map?)?['sessionUpdate'] ==
              'usage_update',
        )
        .take(1)
        .toList();
    final usage =
        ((updates[0]['params'] as Map)['update'] as Map<String, Object?>);
    expect(usage['size'], 128000);
    expect(usage['used'], 900);
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test(
    'emits post-compaction usage_update after automatic compaction',
    () async {
      final wire = await Wire.open(
        // One kept turn leaves the first turn compactable at the end of the
        // second turn, whose reported usage exceeds the 80% threshold.
        keptRecentTurns: 1,
        responses: [
          ...defaultWireResponses(),
          ModelResponse(
            content: const [TextContent('I will inspect.')],
            toolCalls: [
              ToolCall(
                id: ToolCallId('call-2'),
                name: 'read',
                arguments: <String, Object?>{'path': '.'},
              ),
            ],
            stopReason: StopReason.toolUse,
          ),
          const ModelResponse(
            content: [TextContent('Done.')],
            stopReason: StopReason.endTurn,
            usage: TokenUsage(inputTokens: 200000, totalTokens: 200000),
          ),
          const ModelResponse(
            content: [TextContent('Summary.')],
            stopReason: StopReason.endTurn,
          ),
        ],
      );
      final sessionId = await createWireSession(wire);
      await runWirePrompt(wire, sessionId);

      final updates = <Map<String, Object?>>[];
      final sub = wire.turnNotifications.listen(updates.add);
      final promptFuture = wire.send({
        'jsonrpc': '2.0',
        'id': 3,
        'method': 'session/prompt',
        'params': {
          'sessionId': sessionId,
          'prompt': [
            {'type': 'text', 'text': 'Summarize'},
          ],
        },
      });
      final response = await promptFuture;
      await sub.cancel();
      final usages = updates
          .map((m) => (m['params'] as Map)['update'] as Map<String, Object?>)
          .where((u) => u['sessionUpdate'] == 'usage_update')
          .toList();
      // The mid-turn report shows the pre-compaction usage (200000); the
      // post-compaction estimate then replaces it with the freed figure.
      expect(usages, hasLength(2));
      expect(usages.first['used'], 200000);
      expect(usages.last['used'], greaterThan(0));
      expect(usages.last['used'], lessThan(200000));
      expect(usages.last['size'], 128000);
      expect((response['result'] as Map)['stopReason'], 'end_turn');
      await wire.close();
    },
  );
}

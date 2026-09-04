import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'acp_support.dart';

void main() {
  test('set_config_option switches the model for subsequent prompts', () async {
    final wire = await Wire.open(
      models: testCatalog,
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    final set = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/set_config_option',
      'params': {
        'sessionId': sessionId,
        'configId': 'model',
        'value': 'test/m2',
      },
    });
    final options = (set['result'] as Map)['configOptions'] as List<Object?>;
    // m2 declares no reasoning efforts, so the option reverts to one entry.
    expect((options[0] as Map)['currentValue'], 'test/m2');
    expect(options, hasLength(1));

    final prompt = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Inspect the files'},
        ],
      },
    });
    final response = await prompt;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    expect(wire.lastRequest!.model, testModel2);
    expect(wire.lastRequest!.reasoningEffort, isNull);
    await wire.close();
  });

  test('switching models emits usage_update with the new model size', () async {
    final wire = await Wire.open(
      models: testCatalog,
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
          usage: TokenUsage(totalTokens: 1200),
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
          {'type': 'text', 'text': 'Summarize'},
        ],
      },
    });
    // agent_message_chunk, session_info_update, usage_update.
    await wire.turnNotifications.take(3).toList();
    await promptFuture;
    // Subscribe before the switch so the broadcast notification is captured.
    final switchUsage = wire.notifications
        .where(
          (message) =>
              ((message['params'] as Map?)?['update']
                  as Map?)?['sessionUpdate'] ==
              'usage_update',
        )
        .first;
    await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/set_config_option',
      'params': {
        'sessionId': sessionId,
        'configId': 'model',
        'value': 'test/m2',
      },
    });
    final update = (await switchUsage)['params'] as Map;
    final usage = update['update'] as Map<String, Object?>;
    expect(usage['size'], 64000);
    expect(usage['used'], greaterThan(0));
    await wire.close();
  });

  test('usage_update size follows the selected model', () async {
    final wire = await Wire.open(
      models: testCatalog,
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
          usage: TokenUsage(totalTokens: 1200),
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/set_config_option',
      'params': {
        'sessionId': sessionId,
        'configId': 'model',
        'value': 'test/m2',
      },
    });
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
    final usageMessage = await wire.turnNotifications
        .where(
          (message) =>
              ((message['params'] as Map?)?['update']
                  as Map?)?['sessionUpdate'] ==
              'usage_update',
        )
        .first;
    await promptFuture;
    final usage =
        ((usageMessage['params'] as Map)['update'] as Map<String, Object?>);
    expect(usage['used'], 1200);
    expect(usage['size'], 64000);
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('set_config_option switches the reasoning effort', () async {
    final wire = await Wire.open(
      models: testCatalog,
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    final set = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/set_config_option',
      'params': {'sessionId': sessionId, 'configId': 'effort', 'value': 'high'},
    });
    final options = (set['result'] as Map)['configOptions'] as List<Object?>;
    expect(((options[1] as Map)['currentValue']), 'high');

    final prompt = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Inspect the files'},
        ],
      },
    });
    final response = await prompt;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    expect(wire.lastRequest!.model, testModel);
    expect(wire.lastRequest!.reasoningEffort, 'high');
    await wire.close();
  });
  test(
    'set_config_option resets effort when the new model lacks support',
    () async {
      final wire = await Wire.open(
        models: testCatalog,
        responses: [
          const ModelResponse(
            content: [TextContent('Done.')],
            stopReason: StopReason.endTurn,
          ),
        ],
      );
      final sessionId = await createWireSession(wire);
      await wire.send({
        'jsonrpc': '2.0',
        'id': 2,
        'method': 'session/set_config_option',
        'params': {
          'sessionId': sessionId,
          'configId': 'model',
          'value': 'test/m2',
        },
      });
      final back = await wire.send({
        'jsonrpc': '2.0',
        'id': 3,
        'method': 'session/set_config_option',
        'params': {
          'sessionId': sessionId,
          'configId': 'model',
          'value': 'test/m',
        },
      });
      final options = (back['result'] as Map)['configOptions'] as List<Object?>;
      // The previous effort no longer applies; the model's first effort wins.
      expect(((options[1] as Map)['currentValue']), 'low');
      await wire.close();
    },
  );
  test('set_config_option rejects unknown values and option ids', () async {
    final wire = await Wire.open(models: testCatalog);
    final sessionId = await createWireSession(wire);
    for (final params in [
      {'configId': 'model', 'value': 'test/nope'},
      {'configId': 'effort', 'value': 'ultra'},
      {'configId': 'theme', 'value': 'dark'},
    ]) {
      final response = await wire.send({
        'jsonrpc': '2.0',
        'id': 2,
        'method': 'session/set_config_option',
        'params': {'sessionId': sessionId, ...params},
      });
      expect((response['error'] as Map)['code'], -32602);
    }
    await wire.close();
  });
  test('set_config_option rejects unknown sessions', () async {
    final wire = await Wire.open(models: testCatalog);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/set_config_option',
      'params': {
        'sessionId': 'missing',
        'configId': 'model',
        'value': 'test/m',
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    expect(error['message'], contains('session not found'));
    await wire.close();
  });
}

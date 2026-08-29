import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'acp_support.dart';

void main() {
  test('prompt streams updates and completes with end_turn', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);

    final promptFuture = wire.send({
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

    final updates = await wire.turnNotifications.take(5).toList();
    expect(
      updates.map(
        (u) => ((u['params'] as Map)['update'] as Map)['sessionUpdate'],
      ),
      [
        'agent_message_chunk',
        'tool_call',
        'tool_call_update',
        'tool_call_update',
        'agent_message_chunk',
      ],
    );
    // Every notification must be a well-formed JSON-RPC message carrying the
    // session/update method; a bare params object breaks ACP clients.
    for (final update in updates) {
      expect(update['jsonrpc'], '2.0');
      expect(update['method'], 'session/update');
      final params = update['params'] as Map<String, Object?>;
      expect(params['sessionId'], sessionId);
      expect(params['update'], isNotNull);
    }
    final toolCall =
        ((updates[1]['params'] as Map)['update'] as Map<String, Object?>);
    expect(toolCall['toolCallId'], 'call-1');
    expect(toolCall['kind'], 'read');
    // The title is a short human-readable phrase, not the model-facing
    // description or the tool name.
    expect(toolCall['title'], 'Read: .');
    expect(toolCall['rawInput'], {'path': '.'});
    // The path argument is reported as an absolute file location for
    // follow-along, resolved against the session cwd.
    expect(toolCall['locations'], [
      {'path': '/tmp/project/.'},
    ]);
    final completed =
        ((updates[3]['params'] as Map)['update'] as Map<String, Object?>);
    expect(completed['status'], 'completed');
    expect(completed['rawOutput'], {'output': 'file list'});

    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test(
    'cancel stops a running prompt with the cancelled stop reason',
    () async {
      final wire = await Wire.open(blockingProvider: true);
      final sessionId = await createWireSession(wire);

      final promptFuture = wire.send({
        'jsonrpc': '2.0',
        'id': 3,
        'method': 'session/prompt',
        'params': {
          'sessionId': sessionId,
          'prompt': [
            {'type': 'text', 'text': 'Block forever'},
          ],
        },
      });
      await wire.turnNotifications.first;
      wire.sendNotification({
        'jsonrpc': '2.0',
        'method': 'session/cancel',
        'params': {'sessionId': sessionId},
      });

      final response = await promptFuture;
      expect((response['result'] as Map)['stopReason'], 'cancelled');
      await wire.close();
    },
  );
  test('unknown session returns invalid params', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/load',
      'params': {'sessionId': 'missing', 'cwd': '/tmp'},
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    await wire.close();
  });
  test('serializes concurrent prompts on one session', () async {
    final wire = await Wire.open(
      responses: [...defaultWireResponses(), ...defaultWireResponses()],
    );
    final sessionId = await createWireSession(wire);

    final first = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'First turn'},
        ],
      },
    });
    // Wait until the first turn is provably running before queueing another.
    await wire.turnNotifications.first;
    final second = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Second turn'},
        ],
      },
    });

    final firstResponse = await first;
    final secondResponse = await second;
    expect((firstResponse['result'] as Map)['stopReason'], 'end_turn');
    expect((secondResponse['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('rejects unsupported prompt content types', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'audio', 'data': 'AAAA', 'mimeType': 'audio/mpeg'},
        ],
      },
    });
    expect((response['error'] as Map)['code'], -32602);
    await wire.close();
  });
  test('accepts resource link prompt blocks and runs the turn', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {
            'type': 'resource_link',
            'uri': 'file:///tmp/project/main.dart',
            'name': 'main.dart',
          },
          {'type': 'text', 'text': 'Inspect the files'},
        ],
      },
    });
    final updates = await wire.turnNotifications.take(5).toList();
    expect(updates, hasLength(5));
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('prompt with an unknown session returns invalid params', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': 'missing',
        'prompt': [
          {'type': 'text', 'text': 'Hello'},
        ],
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    expect(error['message'], contains('session not found'));
    await wire.close();
  });
  test('prompt with an empty session id returns invalid params', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': '',
        'prompt': [
          {'type': 'text', 'text': 'Hello'},
        ],
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    final data = error['data'] as Map<String, Object?>?;
    expect(data?['stack'], isNull);
    expect(data?['full'], isNull);
    await wire.close();
  });
  test(
    'prompt reports max_tokens when the model hits the token limit',
    () async {
      final wire = await Wire.open(
        responses: [
          const ModelResponse(
            content: [TextContent('Truncated.')],
            stopReason: StopReason.maxTokens,
          ),
        ],
      );
      final sessionId = await createWireSession(wire);
      final response = await wire.send({
        'jsonrpc': '2.0',
        'id': 2,
        'method': 'session/prompt',
        'params': {
          'sessionId': sessionId,
          'prompt': [
            {'type': 'text', 'text': 'Long output please'},
          ],
        },
      });
      expect((response['result'] as Map)['stopReason'], 'max_tokens');
      await wire.close();
    },
  );
  test('EOF does not drop the in-flight prompt response', () async {
    final wire = await Wire.open(
      providerDelay: const Duration(milliseconds: 50),
    );
    final sessionId = await createWireSession(wire);
    final prompt = wire.send({
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
    // Close stdin while the turn is still running; the response must still
    // be delivered before the connection shuts down.
    await wire.closeInput();
    final response = await prompt;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('session/delete cancels the active turn', () async {
    final wire = await Wire.open(blockingProvider: true);
    final sessionId = await createWireSession(wire);

    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Block forever'},
        ],
      },
    });
    await wire.turnNotifications.first;
    final deleted = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/delete',
      'params': {'sessionId': sessionId},
    });
    expect(deleted['result'], <String, Object?>{});
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'cancelled');
    await wire.close();
  });
  test('accepts image prompt blocks and runs the turn', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'image', 'data': 'iVBORw0KGgo=', 'mimeType': 'image/png'},
          {'type': 'text', 'text': 'What is in this image?'},
        ],
      },
    });
    final updates = await wire.turnNotifications.take(5).toList();
    expect(updates, hasLength(5));
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('rejects an image block without mimeType', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'image', 'data': 'iVBORw0KGgo='},
        ],
      },
    });
    expect((response['error'] as Map)['code'], -32602);
    await wire.close();
  });
  test('accepts embedded resource prompt blocks and runs the turn', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {
            'type': 'resource',
            'resource': {
              'uri': 'file:///tmp/project/main.dart',
              'mimeType': 'text/x-dart',
              'text': 'void main() {}',
            },
          },
          {'type': 'text', 'text': 'Review this file'},
        ],
      },
    });
    final updates = await wire.turnNotifications.take(5).toList();
    expect(updates, hasLength(5));
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('rejects a resource block without a uri', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {
            'type': 'resource',
            'resource': {'text': 'no uri'},
          },
        ],
      },
    });
    expect((response['error'] as Map)['code'], -32602);
    await wire.close();
  });
  test('session/close cancels the active turn', () async {
    final wire = await Wire.open(blockingProvider: true);
    final sessionId = await createWireSession(wire);

    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Block forever'},
        ],
      },
    });
    await wire.turnNotifications.first;
    final closed = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/close',
      'params': {'sessionId': sessionId},
    });
    expect(closed['result'], <String, Object?>{});
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'cancelled');
    await wire.close();
  });
}

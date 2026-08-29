import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'acp_support.dart';

void main() {
  test('session/new advertises compact and skill commands', () async {
    final wire = await Wire.open(
      skills: [
        testSkill('review', 'Review the code'),
        // Collides with the built-in command and is filtered out.
        testSkill('compact', 'Duplicate'),
        // Not representable as a slash command and is filtered out.
        testSkill('bad name', 'Spaces'),
      ],
    );
    final created = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {'cwd': '/tmp/project'},
    });
    final sessionId = (created['result'] as Map)['sessionId'] as String;
    final commands = await wireAvailableCommands(wire, sessionId);
    expect(commands.map((command) => command['name']), ['compact', 'review']);
    expect(commands[0]['description'], 'Compact earlier conversation context.');
    expect(commands[1]['description'], 'Review the code');
    expect((commands[1]['input'] as Map)['hint'], 'task');
    await wire.close();
  });
  test(
    'session/load and session/resume advertise available commands',
    () async {
      final wire = await Wire.open(
        skills: [testSkill('review', 'Review the code')],
      );
      final sessionId = await createWireSession(wire);
      for (final method in ['session/load', 'session/resume']) {
        await wire.send({
          'jsonrpc': '2.0',
          'id': 2,
          'method': method,
          'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
        });
        final commands = await wireAvailableCommands(wire, sessionId);
        expect(commands.map((command) => command['name']), [
          'compact',
          'review',
        ]);
      }
      await wire.close();
    },
  );
  test('slash skill command injects the skill into the turn', () async {
    final wire = await Wire.open(
      skills: [testSkill('review', 'Review the code')],
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    final prompt = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': '/review inspect the layout'},
        ],
      },
    });
    final response = await prompt;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    // The skill instructions are injected as a non-persistent context message.
    final messages = wire.lastRequest!.messages;
    expect(
      messages.any(
        (message) => message.content.any(
          (part) =>
              part is TextContent && part.text.contains('<name>review</name>'),
        ),
      ),
      isTrue,
    );
    await wire.close();
  });
  test('unknown slash token falls through to a normal turn', () async {
    final wire = await Wire.open(
      skills: [testSkill('review', 'Review the code')],
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    final prompt = wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': '/nope tell me a joke'},
        ],
      },
    });
    final response = await prompt;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    // No skill context is injected for an unknown token.
    final messages = wire.lastRequest!.messages;
    expect(
      messages.any(
        (message) => message.content.any(
          (part) => part is TextContent && part.text.contains('review'),
        ),
      ),
      isFalse,
    );
    await wire.close();
  });
  test('/compact runs a manual compaction without a model turn', () async {
    final wire = await Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ...defaultWireResponses(),
        ...defaultWireResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);
    await runWirePrompt(wire, sessionId);

    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    final message = await wire.turnNotifications.first;
    final update = (message['params'] as Map)['update'] as Map<String, Object?>;
    expect(update['sessionUpdate'], 'agent_message_chunk');
    expect(
      (update['content'] as Map)['text'],
      startsWith('Context compacted.'),
    );
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    // A bare /compact carries no instruction into the summary request.
    final summaryPrompt = textFromContent(
      wire.lastRequest!.messages.single.content,
    );
    expect(summaryPrompt, isNot(contains('Additional user instruction')));
    await wire.close();
  });
  test('/compact forwards an instruction to the summary request', () async {
    final wire = await Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ...defaultWireResponses(),
        ...defaultWireResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);
    await runWirePrompt(wire, sessionId);

    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': '/compact keep files'},
        ],
      },
    });
    final message = await wire.turnNotifications.first;
    final update = (message['params'] as Map)['update'] as Map<String, Object?>;
    expect(update['sessionUpdate'], 'agent_message_chunk');
    expect(
      (update['content'] as Map)['text'],
      startsWith('Context compacted.'),
    );
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    final summaryPrompt = textFromContent(
      wire.lastRequest!.messages.single.content,
    );
    expect(summaryPrompt, contains('Additional user instruction:\nkeep files'));
    await wire.close();
  });
  test('/compact with nothing to compact reports it', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);

    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    final message = await wire.turnNotifications.first;
    final update = (message['params'] as Map)['update'] as Map<String, Object?>;
    expect(update['sessionUpdate'], 'agent_message_chunk');
    expect((update['content'] as Map)['text'], 'No safe context to compact.');
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('/compact with an image is rejected', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'image', 'data': 'iVBORw0KGgo=', 'mimeType': 'image/png'},
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    expect(error['message'], contains('slash commands do not support images'));
    await wire.close();
  });
  test('/compact with an unknown session returns invalid params', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': 'missing',
        'prompt': [
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    expect(error['message'], contains('session not found'));
    await wire.close();
  });
  test('/compact cancelled by session/delete reports cancelled', () async {
    final wire = await Wire.open(
      // One kept turn leaves the first turn compactable; the provider delay
      // keeps the summary call in flight while the session is deleted.
      keptRecentTurns: 1,
      providerDelay: const Duration(seconds: 2),
      delayedResponseIndex: 4,
      responses: [
        ...defaultWireResponses(),
        ...defaultWireResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);
    await runWirePrompt(wire, sessionId);

    final stopwatch = Stopwatch()..start();
    final promptFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    // Let the compaction summary call enter its provider delay, then delete
    // the session so the turn is cancelled instead of completing.
    await Future<void>.delayed(const Duration(milliseconds: 30));
    await wire.send({
      'jsonrpc': '2.0',
      'id': 4,
      'method': 'session/delete',
      'params': {'sessionId': sessionId},
    });
    final response = await promptFuture;
    stopwatch.stop();
    expect((response['result'] as Map)['stopReason'], 'cancelled');
    expect(stopwatch.elapsed, lessThan(const Duration(seconds: 1)));
    await wire.close();
  });
  test('/compact uses a unique message id per invocation', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final ids = <String>[];
    for (var i = 0; i < 2; i++) {
      final promptFuture = wire.send({
        'jsonrpc': '2.0',
        'id': 2 + i,
        'method': 'session/prompt',
        'params': {
          'sessionId': sessionId,
          'prompt': [
            {'type': 'text', 'text': '/compact'},
          ],
        },
      });
      final message = await wire.turnNotifications.first;
      final update =
          (message['params'] as Map)['update'] as Map<String, Object?>;
      expect(update['sessionUpdate'], 'agent_message_chunk');
      ids.add(update['messageId']! as String);
      await promptFuture;
    }
    expect(ids, hasLength(2));
    expect(ids[0], isNot(ids[1]));
    await wire.close();
  });
  test('/compact emits usage_update with the post-compaction usage', () async {
    final wire = await Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ...defaultWireResponses(),
        ...defaultWireResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);
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
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    final response = await promptFuture;
    await sub.cancel();
    final updatesList = updates
        .map((m) => (m['params'] as Map)['update'] as Map<String, Object?>)
        .toList();
    final chunk = updatesList.firstWhere(
      (u) => u['sessionUpdate'] == 'agent_message_chunk',
    );
    expect((chunk['content'] as Map)['text'], startsWith('Context compacted.'));
    final usages = updatesList
        .where((u) => u['sessionUpdate'] == 'usage_update')
        .toList();
    expect(usages, hasLength(1));
    expect(usages.single['used'], greaterThan(0));
    expect(usages.single['used'], lessThan(200000));
    expect(usages.single['size'], 128000);
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
  test('/compact failure emits no usage_update', () async {
    final wire = await Wire.open(
      // One kept turn leaves the first turn compactable; the empty summary
      // response makes the runtime report CompactionFailed.
      keptRecentTurns: 1,
      responses: [
        ...defaultWireResponses(),
        ...defaultWireResponses(),
        const ModelResponse(content: [], stopReason: StopReason.endTurn),
      ],
    );
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);
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
          {'type': 'text', 'text': '/compact'},
        ],
      },
    });
    final response = await promptFuture;
    await sub.cancel();
    final updatesList = updates
        .map((m) => (m['params'] as Map)['update'] as Map<String, Object?>)
        .toList();
    final chunk = updatesList.firstWhere(
      (u) => u['sessionUpdate'] == 'agent_message_chunk',
    );
    expect((chunk['content'] as Map)['text'], 'Compaction failed.');
    expect(
      updatesList.where((u) => u['sessionUpdate'] == 'usage_update'),
      isEmpty,
    );
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });
}

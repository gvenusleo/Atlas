import 'dart:async';
import 'dart:convert';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'acp_support.dart';

void main() {
  test('session/new, load, and resume reject non-empty mcpServers', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final mcpServers = <Map<String, Object?>>[
      {'name': 'fs', 'command': '/usr/bin/server', 'args': [], 'env': []},
    ];
    for (final method in ['session/new', 'session/load', 'session/resume']) {
      final response = await wire.send({
        'jsonrpc': '2.0',
        'id': 2,
        'method': method,
        'params': {
          'cwd': '/tmp/project',
          'sessionId': sessionId,
          'mcpServers': mcpServers,
        },
      });
      final error = response['error'] as Map<String, Object?>;
      expect(error['code'], -32602);
      if (method == 'session/new') {
        expect(error['message'], contains('mcpServers'));
      }
    }
    await wire.close();
  });

  test('session/new accepts empty or non-array mcpServers as empty', () async {
    // acpd degrades malformed mcpServers to an empty list instead of
    // rejecting the request; the session still creates successfully.
    final wire = await Wire.open();
    for (final mcpServers in [<Object?>[], 'stdio'] as List<Object?>) {
      final response = await wire.send({
        'jsonrpc': '2.0',
        'id': 1,
        'method': 'session/new',
        'params': {'cwd': '/tmp/project', 'mcpServers': mcpServers},
      });
      expect((response['result'] as Map)['sessionId'], isNotEmpty);
    }
    await wire.close();
  });

  test('initialize advertises v1 capabilities', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'initialize',
      'params': {
        'protocolVersion': 1,
        'clientCapabilities': <String, Object?>{},
        'clientInfo': {'name': 'test', 'version': '1.0.0'},
      },
    });
    final result = response['result'] as Map<String, Object?>;
    expect(result['protocolVersion'], 1);
    final capabilities = result['agentCapabilities'] as Map<String, Object?>;
    final meta = capabilities['_meta'] as Map<String, Object?>;
    expect(
      (meta['atlas.dev'] as Map<String, Object?>)['permissionModel'],
      'none',
    );
    expect(capabilities['loadSession'], isTrue);
    final sessionCaps =
        capabilities['sessionCapabilities'] as Map<String, Object?>;
    expect(
      sessionCaps.keys,
      containsAll([
        'resume',
        'list',
        'close',
        'delete',
        'additionalDirectories',
      ]),
    );
    final promptCaps =
        capabilities['promptCapabilities'] as Map<String, Object?>;
    expect(promptCaps['image'], isTrue);
    expect(promptCaps['embeddedContext'], isTrue);
    final info = result['agentInfo'] as Map<String, Object?>;
    expect(info['name'], 'atlas');
    // acpd omits an empty authMethods array on the wire.
    expect(result['authMethods'] ?? const [], isEmpty);
  });
  test('session/new creates a loadable session', () async {
    final wire = await Wire.open();
    final created = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {'cwd': '/tmp/project'},
    });
    final sessionId = (created['result'] as Map)['sessionId'] as String;
    expect(sessionId, isNotEmpty);

    final loaded = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/load',
      'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
    });
    expect(loaded.containsKey('result'), isTrue);
    await wire.close();
  });
  test(
    'session/load replays timeline updates then returns config options',
    () async {
      final wire = await Wire.open();
      final sessionId = await createWireSession(wire);
      await runWirePrompt(wire, sessionId);

      final loadFuture = wire.send({
        'jsonrpc': '2.0',
        'id': 3,
        'method': 'session/load',
        'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
      });
      final replay = await wire.turnNotifications.take(5).toList();
      expect(
        replay.map(
          (u) => ((u['params'] as Map)['update'] as Map)['sessionUpdate'],
        ),
        [
          'user_message_chunk',
          'agent_message_chunk',
          'tool_call',
          'tool_call_update',
          'agent_message_chunk',
        ],
      );
      final response = await loadFuture;
      final result = response['result'] as Map<String, Object?>;
      // The catalog is empty, so only the default model option is offered.
      final options = result['configOptions'] as List<Object?>;
      expect(options, hasLength(1));
      expect((options[0] as Map)['id'], 'model');
      await wire.close();
    },
  );
  test('session/resume returns config options without replaying', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);

    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/resume',
      'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
    });
    final result = response['result'] as Map<String, Object?>;
    expect(result['configOptions'], isNotEmpty);
    // The turn's five updates plus the auto-generated title notification and
    // the usage report emitted on resume.
    expect(wire.notificationCount, 7);
    await wire.close();
  });
  test('session/list returns created sessions', () async {
    final wire = await Wire.open();
    await createWireSession(wire);

    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/list',
      'params': <String, Object?>{},
    });
    final sessions = (response['result'] as Map)['sessions'] as List<Object?>;
    expect(sessions, hasLength(1));
    final first = sessions.single as Map<String, Object?>;
    expect(first['cwd'], '/tmp/project');
    expect(first.containsKey('title'), isFalse);
    expect(first['updatedAt'], isA<String>());
    await wire.close();
  });
  test('session/list reports additional directories', () async {
    final wire = await Wire.open();
    final created = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {
        'cwd': '/tmp/project',
        'additionalDirectories': ['/tmp/shared'],
      },
    });
    expect(created.containsKey('result'), isTrue);

    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/list',
      'params': <String, Object?>{},
    });
    final sessions = (response['result'] as Map)['sessions'] as List<Object?>;
    final first = sessions.single as Map<String, Object?>;
    expect(first['additionalDirectories'], ['/tmp/shared']);
    await wire.close();
  });
  test('session/delete removes a session from session/list', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/delete',
      'params': {'sessionId': sessionId},
    });
    expect(response['result'], <String, Object?>{});
    final list = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/list',
      'params': <String, Object?>{},
    });
    expect((list['result'] as Map)['sessions'] as List<Object?>, isEmpty);
    await wire.close();
  });
  test('session/delete of an unknown session succeeds silently', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/delete',
      'params': {'sessionId': 'missing'},
    });
    expect(response['result'], <String, Object?>{});
    await wire.close();
  });
  test('session/set_title renames a session and lists the new title', () async {
    final wire = await Wire.open();
    final sessionId = await createWireSession(wire);
    final renamed = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': acpSessionSetTitleMethod,
      'params': {'sessionId': sessionId, 'title': 'Renamed title'},
    });
    expect(renamed['result'], <String, Object?>{});
    final list = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/list',
      'params': <String, Object?>{},
    });
    final sessions = (list['result'] as Map)['sessions'] as List<Object?>;
    expect((sessions.single as Map)['title'], 'Renamed title');
    await wire.close();
  });
  test(
    'session/new returns model and reasoning effort config options',
    () async {
      final wire = await Wire.open(models: testCatalog);
      final created = await wire.send({
        'jsonrpc': '2.0',
        'id': 1,
        'method': 'session/new',
        'params': {'cwd': '/tmp/project'},
      });
      final result = created['result'] as Map<String, Object?>;
      final options = result['configOptions'] as List<Object?>;
      expect(options, hasLength(2));
      final modelOption = options[0] as Map<String, Object?>;
      expect(modelOption['id'], 'model');
      expect(modelOption['category'], 'model');
      expect(modelOption['type'], 'select');
      expect(modelOption['currentValue'], 'test/m');
      final modelValues = modelOption['options'] as List<Object?>;
      expect(modelValues, hasLength(2));
      expect((modelValues[0] as Map)['value'], 'test/m');
      expect((modelValues[0] as Map)['name'], 'Model One');
      expect((modelValues[1] as Map)['value'], 'test/m2');
      final effortOption = options[1] as Map<String, Object?>;
      expect(effortOption['id'], 'effort');
      expect(effortOption['category'], 'thought_level');
      expect(effortOption['currentValue'], 'low');
      final effortValues = effortOption['options'] as List<Object?>;
      expect(effortValues, hasLength(2));
      await wire.close();
    },
  );
  test(
    'session/new with an empty catalog still offers the default model',
    () async {
      final wire = await Wire.open();
      final created = await wire.send({
        'jsonrpc': '2.0',
        'id': 1,
        'method': 'session/new',
        'params': {'cwd': '/tmp/project'},
      });
      final result = created['result'] as Map<String, Object?>;
      final options = result['configOptions'] as List<Object?>;
      final modelOption = options[0] as Map<String, Object?>;
      expect(modelOption['currentValue'], 'test/m');
      // The current value must always have a matching selectable option.
      final values = modelOption['options'] as List<Object?>;
      expect(values, hasLength(1));
      expect((values.single as Map)['value'], 'test/m');
      await wire.close();
    },
  );
  test('session/load and session/resume return config options', () async {
    final wire = await Wire.open(models: testCatalog);
    final sessionId = await createWireSession(wire);
    for (final method in ['session/load', 'session/resume']) {
      final response = await wire.send({
        'jsonrpc': '2.0',
        'id': 2,
        'method': method,
        'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
      });
      final result = response['result'] as Map<String, Object?>;
      expect(result['configOptions'] as List<Object?>, hasLength(2));
    }
    await wire.close();
  });
  test('session/load reports the last usage without a turn', () async {
    final wire = await Wire.open(
      responses: [
        ModelResponse(
          content: const [TextContent('I will inspect.')],
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'read',
              arguments: <String, Object?>{'path': '.'},
            ),
          ],
          stopReason: StopReason.toolUse,
        ),
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
          usage: TokenUsage(inputTokens: 1200, totalTokens: 1200),
        ),
      ],
    );
    final sessionId = await createWireSession(wire);
    await runWirePrompt(wire, sessionId);

    // A fresh usage_update is expected on load, so the one from the turn
    // above must not be confused with it.
    final updates = <Map<String, Object?>>[];
    final sub = wire.turnNotifications.listen(updates.add);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/load',
      'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
    });
    expect(response.containsKey('result'), isTrue);
    await sub.cancel();
    final usages = updates
        .map((m) => (m['params'] as Map)['update'] as Map<String, Object?>)
        .where((u) => u['sessionUpdate'] == 'usage_update')
        .toList();
    expect(usages, hasLength(1));
    // The runtime does not persist per-turn usage, so loading estimates the
    // timeline instead: a small positive figure for the single mock turn.
    expect(usages.single['used'], greaterThan(0));
    expect(usages.single['used'], lessThan(1200));
    expect(usages.single['size'], 128000);
    await wire.close();
  });
  test('session/load reports the post-compaction usage', () async {
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
    final compactFuture = wire.send({
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
    await compactFuture;

    final updates = <Map<String, Object?>>[];
    final sub = wire.turnNotifications.listen(updates.add);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 4,
      'method': 'session/load',
      'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
    });
    expect(response.containsKey('result'), isTrue);
    await sub.cancel();
    final usages = updates
        .map((m) => (m['params'] as Map)['update'] as Map<String, Object?>)
        .where((u) => u['sessionUpdate'] == 'usage_update')
        .toList();
    expect(usages, hasLength(1));
    // The checkpoint estimate (summary + kept window) is small; the loaded
    // session reports it instead of the turn's pre-compaction occupancy.
    expect(usages.single['used'], greaterThan(0));
    expect(usages.single['used'], lessThan(200000));
    expect(usages.single['size'], 128000);
    await wire.close();
  });
  test('session/new rejects a relative cwd', () async {
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {'cwd': 'relative/path'},
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    expect(error['message'], contains('absolute path'));
    await wire.close();
  });
  test('session/new ignores non-string additional directories', () async {
    // acpd drops malformed entries instead of rejecting the request.
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {
        'cwd': '/tmp/project',
        'additionalDirectories': [123],
      },
    });
    expect((response['result'] as Map)['sessionId'], isNotEmpty);
    await wire.close();
  });
  test('session/list rejects a non-string cwd filter', () async {
    // acpd fails parsing a non-string cwd, surfacing an internal error.
    final wire = await Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/list',
      'params': {'cwd': 123},
    });
    expect((response['error'] as Map)['code'], isNotNull);
    await wire.close();
  });
  test('ndjson channel decodes one message per line', () async {
    final input = StreamController<List<int>>();
    final output = StreamController<String>();
    final channel = ndjsonChannel(input.stream, output.sink);
    final messages = <Object?>[];
    final done = Completer<void>();
    channel.stream.listen(messages.add, onDone: done.complete);

    input
      ..add(utf8.encode(jsonEncode({'jsonrpc': '2.0', 'id': 1, 'method': 'a'})))
      ..add(utf8.encode('\n'))
      ..add(utf8.encode(jsonEncode({'jsonrpc': '2.0', 'id': 2, 'method': 'b'})))
      ..add(utf8.encode('\n'))
      ..close();
    await done.future;
    expect(messages, hasLength(2));
    expect((jsonDecode(messages[0] as String) as Map)['method'], 'a');
    expect((jsonDecode(messages[1] as String) as Map)['method'], 'b');
  });
}

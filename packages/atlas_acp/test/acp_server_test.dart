import 'dart:async';
import 'dart:convert';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:stream_channel/stream_channel.dart';
import 'package:test/test.dart';

void main() {
  test('initialize advertises v1 capabilities', () async {
    final wire = await _Wire.open();
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
    expect(result['authMethods'], isEmpty);
  });

  test('session/new creates a loadable session', () async {
    final wire = await _Wire.open();
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

  test('prompt streams updates and completes with end_turn', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);

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
      final wire = await _Wire.open(blockingProvider: true);
      final sessionId = await _newSession(wire);

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

  test(
    'session/load replays timeline updates then returns config options',
    () async {
      final wire = await _Wire.open();
      final sessionId = await _newSession(wire);
      await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open();
    await _newSession(wire);

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

  test('unknown session returns invalid params', () async {
    final wire = await _Wire.open();
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
    final wire = await _Wire.open(
      responses: [..._defaultResponses(), ..._defaultResponses()],
    );
    final sessionId = await _newSession(wire);

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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
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
    final wire = await _Wire.open();
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

  test('session/new rejects non-empty mcpServers', () async {
    final wire = await _Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {
        'cwd': '/tmp/project',
        'mcpServers': <Map<String, Object?>>[
          {'name': 'fs', 'command': '/usr/bin/server', 'args': [], 'env': []},
        ],
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    expect(error['message'], contains('mcpServers'));
    await wire.close();
  });

  test('session/new accepts an empty mcpServers array', () async {
    final wire = await _Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {'cwd': '/tmp/project', 'mcpServers': <Object?>[]},
    });
    expect(response.containsKey('result'), isTrue);
    await wire.close();
  });

  test('session/new rejects a non-array mcpServers value', () async {
    final wire = await _Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {'cwd': '/tmp/project', 'mcpServers': 'stdio'},
    });
    expect((response['error'] as Map)['code'], -32602);
    await wire.close();
  });

  test('session/load and session/resume reject non-empty mcpServers', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
    final mcpServers = <Map<String, Object?>>[
      {'name': 'fs', 'command': '/usr/bin/server', 'args': [], 'env': []},
    ];
    for (final method in ['session/load', 'session/resume']) {
      final response = await wire.send({
        'jsonrpc': '2.0',
        'id': 2,
        'method': method,
        'params': {
          'sessionId': sessionId,
          'cwd': '/tmp/project',
          'mcpServers': mcpServers,
        },
      });
      expect((response['error'] as Map)['code'], -32602);
    }
    await wire.close();
  });

  test(
    'prompt reports max_tokens when the model hits the token limit',
    () async {
      final wire = await _Wire.open(
        responses: [
          const ModelResponse(
            content: [TextContent('Truncated.')],
            stopReason: StopReason.maxTokens,
          ),
        ],
      );
      final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open(
      providerDelay: const Duration(milliseconds: 50),
    );
    final sessionId = await _newSession(wire);
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

  test('session/list reports additional directories', () async {
    final wire = await _Wire.open();
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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/delete',
      'params': {'sessionId': 'missing'},
    });
    expect(response['result'], <String, Object?>{});
    await wire.close();
  });

  test('session/delete cancels the active turn', () async {
    final wire = await _Wire.open(blockingProvider: true);
    final sessionId = await _newSession(wire);

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

  test(
    'session/new returns model and reasoning effort config options',
    () async {
      final wire = await _Wire.open(models: _catalog);
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
      expect(effortOption['id'], 'reasoning_effort');
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
      final wire = await _Wire.open();
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
    final wire = await _Wire.open(models: _catalog);
    final sessionId = await _newSession(wire);
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

  test('set_config_option switches the model for subsequent prompts', () async {
    final wire = await _Wire.open(
      models: _catalog,
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
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
    expect(wire.lastRequest!.model, _model2);
    expect(wire.lastRequest!.reasoningEffort, isNull);
    await wire.close();
  });

  test('set_config_option switches the reasoning effort', () async {
    final wire = await _Wire.open(
      models: _catalog,
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
    final set = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/set_config_option',
      'params': {
        'sessionId': sessionId,
        'configId': 'reasoning_effort',
        'value': 'high',
      },
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
    expect(wire.lastRequest!.model, _model);
    expect(wire.lastRequest!.reasoningEffort, 'high');
    await wire.close();
  });

  test(
    'set_config_option resets effort when the new model lacks support',
    () async {
      final wire = await _Wire.open(
        models: _catalog,
        responses: [
          const ModelResponse(
            content: [TextContent('Done.')],
            stopReason: StopReason.endTurn,
          ),
        ],
      );
      final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open(models: _catalog);
    final sessionId = await _newSession(wire);
    for (final params in [
      {'configId': 'model', 'value': 'test/nope'},
      {'configId': 'reasoning_effort', 'value': 'ultra'},
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
    final wire = await _Wire.open(models: _catalog);
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

  test('session/new advertises compact and skill commands', () async {
    final wire = await _Wire.open(
      skills: [
        _skill('review', 'Review the code'),
        // Collides with the built-in command and is filtered out.
        _skill('compact', 'Duplicate'),
        // Not representable as a slash command and is filtered out.
        _skill('bad name', 'Spaces'),
      ],
    );
    final created = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {'cwd': '/tmp/project'},
    });
    final sessionId = (created['result'] as Map)['sessionId'] as String;
    final commands = await _availableCommands(wire, sessionId);
    expect(commands.map((command) => command['name']), ['compact', 'review']);
    expect(commands[0]['description'], 'Compact earlier conversation context.');
    expect(commands[1]['description'], 'Review the code');
    expect((commands[1]['input'] as Map)['hint'], 'task');
    await wire.close();
  });

  test(
    'session/load and session/resume advertise available commands',
    () async {
      final wire = await _Wire.open(
        skills: [_skill('review', 'Review the code')],
      );
      final sessionId = await _newSession(wire);
      for (final method in ['session/load', 'session/resume']) {
        await wire.send({
          'jsonrpc': '2.0',
          'id': 2,
          'method': method,
          'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
        });
        final commands = await _availableCommands(wire, sessionId);
        expect(commands.map((command) => command['name']), [
          'compact',
          'review',
        ]);
      }
      await wire.close();
    },
  );

  test('slash skill command injects the skill into the turn', () async {
    final wire = await _Wire.open(
      skills: [_skill('review', 'Review the code')],
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open(
      skills: [_skill('review', 'Review the code')],
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ..._defaultResponses(),
        ..._defaultResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ..._defaultResponses(),
        ..._defaultResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
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
    final wire = await _Wire.open(
      // One kept turn leaves the first turn compactable; the provider delay
      // keeps the summary call in flight while the session is deleted.
      keptRecentTurns: 1,
      providerDelay: const Duration(seconds: 2),
      delayedResponseIndex: 4,
      responses: [
        ..._defaultResponses(),
        ..._defaultResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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

  test('emits session_info_update when a prompt generates a title', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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

  test('accepts image prompt blocks and runs the turn', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
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

  test('emits usage_update with used tokens and context size', () async {
    final wire = await _Wire.open(
      responses: [
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
          usage: TokenUsage(totalTokens: 1200),
        ),
      ],
    );
    final sessionId = await _newSession(wire);
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
    final updates = await wire.turnNotifications.take(3).toList();
    final usage =
        ((updates[2]['params'] as Map)['update'] as Map<String, Object?>);
    expect(usage['sessionUpdate'], 'usage_update');
    expect(usage['used'], 1200);
    expect(usage['size'], 128000);
    final response = await promptFuture;
    expect((response['result'] as Map)['stopReason'], 'end_turn');
    await wire.close();
  });

  test(
    'emits post-compaction usage_update after automatic compaction',
    () async {
      final wire = await _Wire.open(
        // One kept turn leaves the first turn compactable at the end of the
        // second turn, whose reported usage exceeds the 80% threshold.
        keptRecentTurns: 1,
        responses: [
          ..._defaultResponses(),
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
      final sessionId = await _newSession(wire);
      await _runPrompt(wire, sessionId);

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
      // Only the post-compaction usage is reported; the pre-compaction turn
      // usage (200000) is replaced by the compacted estimate.
      expect(usages, hasLength(1));
      expect(usages.single['used'], greaterThan(0));
      expect(usages.single['used'], lessThan(200000));
      expect(usages.single['size'], 128000);
      expect((response['result'] as Map)['stopReason'], 'end_turn');
      await wire.close();
    },
  );

  test('/compact emits usage_update with the post-compaction usage', () async {
    final wire = await _Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ..._defaultResponses(),
        ..._defaultResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open(
      // One kept turn leaves the first turn compactable; the empty summary
      // response makes the runtime report CompactionFailed.
      keptRecentTurns: 1,
      responses: [
        ..._defaultResponses(),
        ..._defaultResponses(),
        const ModelResponse(content: [], stopReason: StopReason.endTurn),
      ],
    );
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);
    await _runPrompt(wire, sessionId);

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

  test('session/load reports the last usage without a turn', () async {
    final wire = await _Wire.open(
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
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);

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
    final wire = await _Wire.open(
      // One kept turn leaves the first turn compactable.
      keptRecentTurns: 1,
      responses: [
        ..._defaultResponses(),
        ..._defaultResponses(),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ],
    );
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);
    await _runPrompt(wire, sessionId);
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
    final wire = await _Wire.open();
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

  test('session/new rejects non-string additional directories', () async {
    final wire = await _Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/new',
      'params': {
        'cwd': '/tmp/project',
        'additionalDirectories': [123],
      },
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    await wire.close();
  });

  test('session/list rejects a non-string cwd filter', () async {
    final wire = await _Wire.open();
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 1,
      'method': 'session/list',
      'params': {'cwd': 123},
    });
    final error = response['error'] as Map<String, Object?>;
    expect(error['code'], -32602);
    await wire.close();
  });

  test('session/close cancels the active turn', () async {
    final wire = await _Wire.open(blockingProvider: true);
    final sessionId = await _newSession(wire);

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

Future<String> _newSession(_Wire wire) async {
  final response = await wire.send({
    'jsonrpc': '2.0',
    'id': 1,
    'method': 'session/new',
    'params': {'cwd': '/tmp/project'},
  });
  return (response['result'] as Map<String, Object?>)['sessionId']! as String;
}

Future<void> _runPrompt(_Wire wire, String sessionId) async {
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
  await wire.turnNotifications.take(5).toList();
  await promptFuture;
}

/// Awaits the latest `available_commands_update` notification for
/// [sessionId] and returns its command list.
Future<List<Map<String, Object?>>> _availableCommands(
  _Wire wire,
  String sessionId,
) async {
  final message = await wire.notifications.where((m) {
    final update = (m['params'] as Map?)?['update'];
    return update is Map &&
        update['sessionUpdate'] == 'available_commands_update' &&
        (m['params'] as Map)['sessionId'] == sessionId;
  }).first;
  final update = (message['params'] as Map)['update'] as Map<String, Object?>;
  return [
    for (final command in update['availableCommands'] as List<Object?>)
      (command as Map).cast<String, Object?>(),
  ];
}

final _model = ModelRef(providerId: ProviderId('test'), modelId: ModelId('m'));
final _model2 = ModelRef(
  providerId: ProviderId('test'),
  modelId: ModelId('m2'),
);

/// Builds a test skill with the given [name] and [description].
Skill _skill(String name, String description) => Skill(
  name: name,
  description: description,
  dir: '/tmp/project/.atlas/skills/$name',
  path: '/tmp/project/.atlas/skills/$name/SKILL.md',
  content: 'Instructions for $name.',
);

/// A model catalog with one reasoning-capable model and one plain model.
final _catalog = <ModelDescriptor>[
  ModelDescriptor(
    ref: _model,
    name: 'Model One',
    description: 'Fast model',
    contextWindow: 128000,
    reasoningEfforts: const [
      ReasoningEffortOption(value: 'low', name: 'Low'),
      ReasoningEffortOption(value: 'high', name: 'High'),
    ],
  ),
  ModelDescriptor(ref: _model2, name: 'Model Two', contextWindow: 64000),
];

/// The default scripted turn: one tool call followed by a final response.
List<ModelResponse> _defaultResponses() => [
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
  ),
  const ModelResponse(
    content: [TextContent('Done.')],
    stopReason: StopReason.endTurn,
  ),
];

/// A JSON-RPC wire harness driving an [AcpServer] over an in-memory channel.
final class _Wire {
  _Wire._(this._requests, this._provider);

  final StreamController<String> _requests;
  final ModelProvider _provider;
  final _pending = <Object?, Completer<Map<String, Object?>>>{};
  final _notifications = StreamController<Map<String, Object?>>.broadcast();
  final _clientRequests = StreamController<Map<String, Object?>>.broadcast();
  var _notificationCount = 0;
  var _clientRequestCount = 0;
  late final Future<void> _serverDone;

  static Future<_Wire> open({
    bool blockingProvider = false,
    List<ModelResponse>? responses,
    Duration providerDelay = Duration.zero,
    int? delayedResponseIndex,
    List<ModelDescriptor> models = const [],
    List<Skill> skills = const [],
    int keptRecentTurns = 5,
    ToolRegistry? tools,
  }) async {
    final provider = blockingProvider
        ? _BlockingProvider()
        : _ScriptedProvider(
            responses ?? _defaultResponses(),
            delay: providerDelay,
            delayedResponseIndex: delayedResponseIndex,
          );
    final runtime = AgentRuntime(
      store: _MemorySessionStore(),
      provider: provider,
      tools: tools ?? _MemoryTools(),
      ids: _Ids(),
      defaultModel: _model,
      maxSteps: 2,
      keptRecentTurns: keptRecentTurns,
      sessionContextBuilder: (cwd) => SessionContext(
        workingDirectory: cwd,
        instructions: const [],
        skills: _MemorySkillCatalog([
          for (final skill in skills)
            SkillSummary(
              name: skill.name,
              path: skill.path,
              description: skill.description,
            ),
        ], skills: skills),
      ),
    );
    final server = AcpServer(runtime, models: models);
    final requests = StreamController<String>();
    final outgoing = StreamController<String>();
    final wire = _Wire._(requests, provider);
    outgoing.stream.listen((line) {
      final message = jsonDecode(line);
      if (message is Map) {
        final map = message.cast<String, Object?>();
        if (map['method'] != null && map.containsKey('id')) {
          // Agent-to-client requests (e.g. terminal/create) must be
          // answered by the harness before the agent proceeds.
          wire._clientRequestCount++;
          wire._clientRequests.add(map);
        } else if (map.containsKey('id')) {
          wire._pending.remove(map['id'])?.complete(map);
        } else {
          // Lifecycle notifications (e.g. available_commands_update) are
          // excluded from the turn notification count.
          final update = (map['params'] as Map?)?['update'];
          final isTurn =
              !(update is Map &&
                  update['sessionUpdate'] == 'available_commands_update');
          if (isTurn) {
            wire._notificationCount++;
          }
          wire._notifications.add(map);
        }
      }
    });
    wire._serverDone = server.serveChannel(
      StreamChannel<String>(requests.stream, outgoing.sink),
    );
    return wire;
  }

  /// Notifications received so far.
  Stream<Map<String, Object?>> get notifications => _notifications.stream;

  /// Turn notifications, excluding lifecycle notifications such as
  /// `available_commands_update` that arrive between requests.
  Stream<Map<String, Object?>> get turnNotifications =>
      _notifications.stream.where((message) {
        final update = (message['params'] as Map?)?['update'];
        return !(update is Map &&
            update['sessionUpdate'] == 'available_commands_update');
      });

  /// The number of notifications received so far.
  int get notificationCount => _notificationCount;

  /// Agent-to-client requests (e.g. `terminal/create`) awaiting a reply.
  Stream<Map<String, Object?>> get clientRequests => _clientRequests.stream;

  /// The number of agent-to-client requests received so far.
  int get clientRequestCount => _clientRequestCount;

  /// Answers an agent-to-client [request] with [result], or with a JSON-RPC
  /// [error] to exercise the client-failure path.
  void respondToRequest(
    Map<String, Object?> request, {
    Map<String, Object?>? result,
    Map<String, Object?>? error,
  }) {
    _requests.add(
      jsonEncode({
        'jsonrpc': '2.0',
        'id': request['id'],
        'result': ?result,
        'error': ?error,
      }),
    );
  }

  /// The model request of the most recent turn, when one was run.
  ModelRequest? get lastRequest => switch (_provider) {
    _ScriptedProvider(:final lastRequest) => lastRequest,
    _ => null,
  };

  /// Sends a request and awaits its response.
  Future<Map<String, Object?>> send(Map<String, Object?> request) {
    final completer = Completer<Map<String, Object?>>();
    _pending[request['id']] = completer;
    _requests.add(jsonEncode(request));
    return completer.future;
  }

  /// Sends a notification without awaiting a response.
  void sendNotification(Map<String, Object?> notification) {
    _requests.add(jsonEncode(notification));
  }

  /// Closes the input (EOF) as a client would when shutting down.
  Future<void> closeInput() async {
    await _requests.close();
  }

  Future<void> close() async {
    await closeInput();
    await _serverDone;
    // Let deferred lifecycle notifications (available_commands_update) flush
    // before the broadcast controller closes.
    await Future<void>.delayed(Duration.zero);
    await _notifications.close();
  }
}

final class _ScriptedProvider implements ModelProvider {
  _ScriptedProvider(
    this.responses, {
    this.delay = Duration.zero,
    this.delayedResponseIndex,
  });

  final List<ModelResponse> responses;

  /// Artificial latency so a turn is still running when a test closes EOF.
  final Duration delay;

  /// Limits [delay] to one response index when set.
  final int? delayedResponseIndex;
  var _index = 0;

  /// The most recent model request, captured for assertions.
  ModelRequest? lastRequest;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, name: 'test', contextWindow: 128000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    lastRequest = request;
    final index = _index++;
    if (index >= responses.length) {
      throw const TurnCancelledException();
    }
    if (delay > Duration.zero &&
        (delayedResponseIndex == null || delayedResponseIndex == index)) {
      await Future.any<void>([
        Future<void>.delayed(delay),
        if (request.cancellation != null) request.cancellation!.whenCancelled,
      ]);
      request.cancellation?.throwIfCancelled();
    }
    final response = responses[index];
    for (final part in response.content) {
      if (part is TextContent) {
        yield TextDeltaEvent(part.text);
      }
    }
    yield ModelCompletedEvent(response);
  }
}

/// A provider that emits one delta and then waits for cancellation.
final class _BlockingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, name: 'test', contextWindow: 128000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield const TextDeltaEvent('thinking');
    await request.cancellation!.whenCancelled;
    throw const TurnCancelledException();
  }
}

/// An in-memory skill catalog returning the injected summaries and skills.
final class _MemorySkillCatalog implements SkillCatalog {
  _MemorySkillCatalog(
    Iterable<SkillSummary> summaries, {
    this.skills = const [],
  }) : _summaries = List.unmodifiable(summaries);

  final List<Skill> skills;
  final List<SkillSummary> _summaries;

  @override
  List<SkillSummary> get summaries => _summaries;

  @override
  Skill? lookup(String name) {
    for (final skill in skills) {
      if (skill.name == name) {
        return skill;
      }
    }
    return null;
  }
}

final class _MemoryTools implements ToolRegistry {
  @override
  List<ToolDescriptor> get descriptors => const [
    ToolDescriptor(
      name: 'read',
      description: 'Read a file',
      inputSchema: <String, Object?>{},
    ),
  ];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async =>
      const ToolResult(content: 'file list');
}

final class _MemorySessionStore implements SessionStore {
  Session? session;
  final turns = <Turn>[];
  final timeline = <TimelineItem>[];
  final checkpoints = <ModelCheckpoint>[];

  @override
  Future<void> createSession(Session value) async => session = value;

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) async {
    final value = session;
    if (value == null || value.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    return SessionSnapshot(
      session: value,
      turns: List.unmodifiable(turns),
      timeline: List.unmodifiable(timeline),
      modelCheckpoints: List.unmodifiable(checkpoints),
    );
  }

  @override
  Future<SessionPage> listSessions(SessionQuery query) async => SessionPage(
    items: [
      if (session != null)
        SessionSummary(
          id: session!.id,
          title: session!.title,
          workingDirectory: session!.workingDirectory,
          additionalDirectories: session!.additionalDirectories,
          updatedAt: session!.updatedAt,
        ),
    ],
  );

  @override
  Future<void> beginTurn(BeginTurn operation) async {
    // Mirror DriftSessionStore: the title is auto-generated from the first
    // user message when the session has none yet.
    var value = operation.session;
    if (value.title.isEmpty) {
      final text = textFromContent(operation.userMessage.content).trim();
      if (text.isNotEmpty) {
        final firstLine = text.split('\n').first.trim();
        value = Session(
          id: value.id,
          title: String.fromCharCodes(firstLine.runes.take(80)),
          workingDirectory: value.workingDirectory,
          additionalDirectories: value.additionalDirectories,
          createdAt: value.createdAt,
          updatedAt: value.updatedAt,
          compaction: value.compaction,
          lastUsage: value.lastUsage,
        );
      }
    }
    session = value;
    turns.add(operation.turn);
    timeline.add(operation.userMessage);
  }

  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) async {
    timeline.add(operation.assistantMessage);
    timeline.addAll(operation.toolCalls);
    if (operation.checkpoint != null) checkpoints.add(operation.checkpoint!);
  }

  @override
  Future<void> appendToolResult(
    SessionId sessionId,
    ToolResultItem item,
  ) async => timeline.add(item);

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) async {
    final index = turns.indexWhere((item) => item.id == turn.id);
    if (index < 0) {
      // Mirror DriftSessionStore: the turn row is gone, e.g. because the
      // session was deleted with ON DELETE CASCADE while the turn ran.
      throw SessionNotFoundException(sessionId);
    }
    turns[index] = turn;
  }

  @override
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  ) async {
    final value = session;
    if (value != null) {
      session = Session(
        id: value.id,
        title: value.title,
        workingDirectory: value.workingDirectory,
        additionalDirectories: value.additionalDirectories,
        createdAt: value.createdAt,
        updatedAt: value.updatedAt,
        compaction: checkpoint,
        lastUsage: value.lastUsage,
      );
    }
  }

  @override
  Future<void> deleteSession(SessionId sessionId) async {
    // Mirror DriftSessionStore: unknown sessions throw, and deleting a
    // session cascades to its turns, messages, and checkpoints.
    if (session == null || session!.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    session = null;
    turns.clear();
    timeline.clear();
    checkpoints.clear();
  }

  @override
  Future<void> renameSession(SessionId sessionId, String title) async {
    final value = session;
    if (value == null || value.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    session = Session(
      id: value.id,
      workingDirectory: value.workingDirectory,
      additionalDirectories: value.additionalDirectories,
      createdAt: value.createdAt,
      updatedAt: value.updatedAt,
      title: title,
      compaction: value.compaction,
      lastUsage: value.lastUsage,
    );
  }
}

final class _Ids implements IdGenerator {
  var _session = 0;
  var _turn = 0;
  var _item = 0;

  @override
  SessionId sessionId() => SessionId('session-${++_session}');

  @override
  TurnId turnId() => TurnId('turn-${++_turn}');

  @override
  TimelineItemId timelineItemId() => TimelineItemId('item-${++_item}');
}

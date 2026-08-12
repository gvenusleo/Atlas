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
      containsAll(['resume', 'list', 'close', 'additionalDirectories']),
    );
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

    final updates = await wire.notifications.take(5).toList();
    expect(updates.map((u) => (u['update'] as Map)['sessionUpdate']), [
      'agent_message_chunk',
      'tool_call',
      'tool_call_update',
      'tool_call_update',
      'agent_message_chunk',
    ]);
    final toolCall = updates[1]['update'] as Map<String, Object?>;
    expect(toolCall['toolCallId'], 'call-1');
    expect(toolCall['kind'], 'read');
    expect((updates[3]['update'] as Map)['status'], 'completed');

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
      await wire.notifications.first;
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

  test('session/load replays timeline updates then returns null', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);

    final loadFuture = wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/load',
      'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
    });
    final replay = await wire.notifications.take(5).toList();
    expect(replay.map((u) => (u['update'] as Map)['sessionUpdate']), [
      'user_message_chunk',
      'agent_message_chunk',
      'tool_call',
      'tool_call_update',
      'agent_message_chunk',
    ]);
    final response = await loadFuture;
    expect(response['result'], isNull);
    await wire.close();
  });

  test('session/resume returns without replaying', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
    await _runPrompt(wire, sessionId);

    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/resume',
      'params': {'sessionId': sessionId, 'cwd': '/tmp/project'},
    });
    expect(response['result'], <String, Object?>{});
    expect(wire.notificationCount, 5);
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

  test('rejects a second prompt while one is active', () async {
    final wire = await _Wire.open(blockingProvider: true);
    final sessionId = await _newSession(wire);

    final first = wire.send({
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
    await wire.notifications.first;
    final second = await wire.send({
      'jsonrpc': '2.0',
      'id': 3,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'text', 'text': 'Again'},
        ],
      },
    });
    expect((second['error'] as Map)['code'], -32603);
    wire.sendNotification({
      'jsonrpc': '2.0',
      'method': 'session/cancel',
      'params': {'sessionId': sessionId},
    });
    await first;
    await wire.close();
  });

  test('rejects non-text prompt content', () async {
    final wire = await _Wire.open();
    final sessionId = await _newSession(wire);
    final response = await wire.send({
      'jsonrpc': '2.0',
      'id': 2,
      'method': 'session/prompt',
      'params': {
        'sessionId': sessionId,
        'prompt': [
          {'type': 'image', 'data': 'iVBOR', 'mimeType': 'image/png'},
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
    final updates = await wire.notifications.take(5).toList();
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
    await wire.notifications.first;
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
  await wire.notifications.take(5).toList();
  await promptFuture;
}

final _model = ModelRef(providerId: ProviderId('test'), modelId: ModelId('m'));

/// A JSON-RPC wire harness driving an [AcpServer] over an in-memory channel.
final class _Wire {
  _Wire._(this._requests);

  final StreamController<String> _requests;
  final _pending = <Object?, Completer<Map<String, Object?>>>{};
  final _notifications = StreamController<Map<String, Object?>>.broadcast();
  var _notificationCount = 0;
  late final Future<void> _serverDone;

  static Future<_Wire> open({bool blockingProvider = false}) async {
    final runtime = AgentRuntime(
      store: _MemorySessionStore(),
      provider: blockingProvider
          ? _BlockingProvider()
          : _ScriptedProvider([
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
            ]),
      tools: _MemoryTools(),
      ids: _Ids(),
      defaultModel: _model,
      maxSteps: 2,
    );
    final server = AcpServer(runtime);
    final requests = StreamController<String>();
    final outgoing = StreamController<String>();
    final wire = _Wire._(requests);
    outgoing.stream.listen((line) {
      final message = jsonDecode(line);
      if (message is Map) {
        final map = message.cast<String, Object?>();
        if (map.containsKey('id')) {
          wire._pending.remove(map['id'])?.complete(map);
        } else {
          wire._notificationCount++;
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

  /// The number of notifications received so far.
  int get notificationCount => _notificationCount;

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

  Future<void> close() async {
    await _requests.close();
    await _serverDone;
    await _notifications.close();
  }
}

final class _ScriptedProvider implements ModelProvider {
  _ScriptedProvider(this.responses);

  final List<ModelResponse> responses;
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, name: 'test', contextWindow: 128000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    final index = _index++;
    if (index >= responses.length) {
      throw const TurnCancelledException();
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
          updatedAt: session!.updatedAt,
        ),
    ],
  );

  @override
  Future<void> beginTurn(BeginTurn operation) async {
    session = operation.session;
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
    turns[turns.indexWhere((item) => item.id == turn.id)] = turn;
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
  Future<void> deleteSession(SessionId sessionId) async => session = null;
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

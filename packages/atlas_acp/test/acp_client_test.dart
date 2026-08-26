import 'dart:async';
import 'dart:convert';

import 'package:acpd/acpd.dart' as acpd;
import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart' as rt;
import 'package:stream_channel/stream_channel.dart';
import 'package:test/test.dart';

void main() {
  test('connect initializes and lists sessions', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final page = await client.listSessions();
    expect(page.items, isEmpty);
    expect(wire.methods, containsAll(['initialize', 'session/list']));
    await wire.close();
  });

  test('initialize sends the full ACP parameter set', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final init = wire.requests.firstWhere(
      (request) => request.method == 'initialize',
    );
    final params = init.params as Map<String, Object?>;
    expect(params['protocolVersion'], 1);
    final info = params['clientInfo'] as Map<String, Object?>;
    expect(info['name'], isNotEmpty);
    await wire.close();
  });

  test('session/new includes the mcpServers parameter', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    await client.createSession(workingDirectory: '/tmp');
    final request = wire.requests.firstWhere(
      (request) => request.method == 'session/new',
    );
    expect((request.params as Map<String, Object?>)['mcpServers'], <Object?>[]);
    await wire.close();
  });

  test('uses a seeded catalog before the first session is created', () async {
    final catalog = [
      rt.ModelDescriptor(
        ref: rt.ModelRef(
          providerId: rt.ProviderId('anthropic'),
          modelId: rt.ModelId('claude-sonnet'),
        ),
        name: 'Claude Sonnet',
        reasoningEfforts: const [rt.ReasoningEffortOption(value: 'high')],
      ),
    ];
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(
      wire.clientChannel,
      catalog: catalog,
      defaultModel: catalog.first.ref,
    );
    await client.connect();
    addTearDown(client.close);

    expect(client.defaultModel, catalog.first.ref);
    expect(client.catalog.single.name, 'Claude Sonnet');
    expect(client.catalog.single.reasoningEfforts.single.value, 'high');
    await wire.close();
  });

  test(
    'parses the model catalog and reasoning efforts from configOptions',
    () async {
      final wire = await _FakeServer.open();
      final client = AcpClient.channel(wire.clientChannel);
      await client.connect();
      addTearDown(client.close);

      await client.createSession(workingDirectory: '/tmp');

      expect(client.catalog, hasLength(2));
      final first = client.catalog.first;
      expect(first.ref.toString(), 'opencode/foo');
      expect(first.name, 'Foo Model');
      expect(first.reasoningEfforts.map((effort) => effort.value), [
        'low',
        'high',
      ]);
      expect(client.defaultModel.toString(), 'opencode/foo');
      await wire.close();
    },
  );

  test('runs a full turn and reconstructs runtime events', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    final events = <rt.AgentEvent>[];
    await for (final event in client.run(
      rt.TurnRequest(
        sessionId: session.id,
        content: const [rt.TextContent('hello')],
      ),
    )) {
      events.add(event);
    }

    expect(events.map((event) => event.runtimeType), [
      rt.TurnStarted,
      rt.PlanUpdated,
      rt.ModelReasoningDelta,
      rt.ModelTextDelta,
      rt.ToolStarted,
      rt.ToolFinished,
      rt.TurnFinished,
    ]);
    final finished = events.last as rt.TurnFinished;
    expect(finished.outcome.status, rt.TurnStatus.completed);
    expect(finished.outcome.stopReason, rt.StopReason.endTurn);
    final plan = events.whereType<rt.PlanUpdated>().first;
    expect(plan.entries, hasLength(2));
    final toolStarted = events[4] as rt.ToolStarted;
    expect(toolStarted.call.call.name, 'read');
    final toolFinished = events[5] as rt.ToolFinished;
    expect(toolFinished.result.content, 'file list');
    await wire.close();
  });

  test('loads a session from a strict server without the cwd field', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    final snapshot = await client.loadSession(session.id);
    expect(snapshot.session.workingDirectory, '/tmp');
    expect(
      snapshot.timeline.map((item) => item.runtimeType),
      containsAll([rt.UserMessageItem, rt.AssistantMessageItem]),
    );
    await wire.close();
  });

  test('loads a session timeline from the replay stream', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    await client
        .run(
          rt.TurnRequest(
            sessionId: session.id,
            content: const [rt.TextContent('hi')],
          ),
        )
        .toList();
    final snapshot = await client.loadSession(session.id);

    expect(snapshot.session.workingDirectory, '/tmp');
    expect(
      snapshot.timeline.map((item) => item.runtimeType),
      containsAll([
        rt.UserMessageItem,
        rt.AssistantMessageItem,
        rt.ToolCallItem,
      ]),
    );
    await wire.close();
  });

  test('cancel stops a running turn with the cancelled stop reason', () async {
    final wire = await _FakeServer.open(
      promptDelay: const Duration(seconds: 2),
    );
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    final cancellation = rt.CancellationToken();
    final events = <rt.AgentEvent>[];
    final runFuture = client
        .run(
          rt.TurnRequest(
            sessionId: session.id,
            content: const [rt.TextContent('hello')],
            cancellation: cancellation,
          ),
        )
        .listen(events.add);
    await Future<void>.delayed(const Duration(milliseconds: 50));
    cancellation.cancel();
    await runFuture.cancel();
    await Future<void>.delayed(const Duration(milliseconds: 100));

    expect(wire.cancelledSessions, contains('sess-1'));
    await wire.close();
  });

  test('compacts a session and reports the checkpoint', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    final events = <rt.AgentEvent>[];
    await for (final event in client.compact(session.id)) {
      events.add(event);
    }

    final finished = events.whereType<rt.CompactionFinished>().single;
    expect(finished.checkpoint.keptRecentMessages, 3);
    expect(finished.checkpoint.inputTokensAfter, 1200);
    await wire.close();
  });

  test('tolerates unknown update kinds from third-party servers', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    wire.pushUnknownUpdate();
    final events = <rt.AgentEvent>[];
    await for (final event in client.run(
      rt.TurnRequest(
        sessionId: session.id,
        content: const [rt.TextContent('x')],
      ),
    )) {
      events.add(event);
    }

    expect(events.whereType<rt.TurnFinished>(), hasLength(1));
    await wire.close();
  });

  test(
    'renameSession keeps a local title overlay when the agent has no rename method',
    () async {
      final wire = await _FakeServer.open();
      final client = AcpClient.channel(wire.clientChannel);
      await client.connect();
      addTearDown(client.close);

      final session = await client.createSession(workingDirectory: '/tmp');
      await client.renameSession(session.id, 'new title');
      expect(client.titleFor(session.id), 'new title');
      expect(wire.methods, isNot(contains(acpSessionSetTitleMethod)));
      await wire.close();
    },
  );

  test('surfaces permission requests and replies to the agent', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final requests = <rt.PermissionRequest>[];
    final sub = client.permissionRequests.listen(requests.add);
    addTearDown(sub.cancel);

    wire.pushPermissionRequest();
    await pumpEventQueue();

    expect(requests, hasLength(1));
    final request = requests.single;
    expect(request.toolCallId, 'call-9');
    expect(request.title, 'Edit: /tmp/a.txt');
    expect(request.options, hasLength(3));

    await client.respondPermission(
      request.requestId,
      rt.PermissionReply.allowOnce,
    );
    await pumpEventQueue();

    expect(wire.permissionOutcomes, hasLength(1));
    final outcome = wire.permissionOutcomes.single;
    expect(outcome, isA<acpd.PermissionSelected>());
    await wire.close();
  });

  test('rejects permission requests when the client closes', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();

    final requests = <rt.PermissionRequest>[];
    final sub = client.permissionRequests.listen(requests.add);
    addTearDown(sub.cancel);

    wire.pushPermissionRequest();
    await pumpEventQueue();
    expect(requests, hasLength(1));
    await client.close();
    await pumpEventQueue();
    expect(wire.permissionOutcomes, hasLength(1));
    final outcome = wire.permissionOutcomes.single;
    expect(outcome, isA<acpd.PermissionSelected>());
    expect((outcome as acpd.PermissionSelected).optionId, 'reject');
    await wire.close();
  });

  test(
    'assigns unique request ids to concurrent permission requests',
    () async {
      final wire = await _FakeServer.open();
      final client = AcpClient.channel(wire.clientChannel);
      await client.connect();
      addTearDown(client.close);

      final requests = <rt.PermissionRequest>[];
      final sub = client.permissionRequests.listen(requests.add);
      addTearDown(sub.cancel);

      wire.pushPermissionRequest();
      wire.pushPermissionRequest();
      await pumpEventQueue();

      expect(requests, hasLength(2));
      expect(requests[0].requestId, isNot(requests[1].requestId));
      await client.respondPermission(
        requests[0].requestId,
        rt.PermissionReply.allowOnce,
      );
      await client.respondPermission(
        requests[1].requestId,
        rt.PermissionReply.reject,
      );
      await pumpEventQueue();
      expect(wire.permissionOutcomes, hasLength(2));
      await wire.close();
    },
  );

  test('falls back to session/close when delete is unsupported', () async {
    final wire = await _FakeServer.open(deleteUnsupported: true);
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    await client.deleteSession(session.id);

    expect(wire.methods, contains('session/close'));
    expect(wire.closedSessions, contains('sess-1'));
    await wire.close();
  });

  test('parses session info, commands, and mode changes', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    wire.pushInfoCommandsAndMode();
    await pumpEventQueue();

    expect(client.titleFor(session.id), 'My session');
    expect(client.commandsFor(session.id).single.name, 'web');
    expect(client.modeFor(session.id), 'build');
    await wire.close();
  });

  test('parses mode options and syncs the mode before a prompt', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    expect(client.modeOptions.map((option) => option.id), ['build', 'plan']);
    expect(client.modeFor(session.id), 'build');

    await client
        .run(
          rt.TurnRequest(
            sessionId: session.id,
            content: const [rt.TextContent('x')],
            mode: 'plan',
          ),
        )
        .toList();

    final modeRequest = wire.requests.lastWhere(
      (request) => request.method == 'session/set_config_option',
    );
    final params = modeRequest.params as Map<String, Object?>;
    expect(params['configId'], 'mode');
    expect(params['value'], 'plan');
    expect(client.modeFor(session.id), 'plan');
    await wire.close();
  });

  test('maps plan updates into rt.PlanUpdated events', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    final session = await client.createSession(workingDirectory: '/tmp');
    final events = <rt.AgentEvent>[];
    await for (final event in client.run(
      rt.TurnRequest(
        sessionId: session.id,
        content: const [rt.TextContent('x')],
      ),
    )) {
      events.add(event);
    }

    final plan = events.whereType<rt.PlanUpdated>().first;
    expect(plan.entries, hasLength(2));
    expect(plan.entries.first.content, 'Check syntax');
    expect(plan.entries.last.status, 'in_progress');
    await wire.close();
  });

  test('exposes auth methods and authenticates', () async {
    final wire = await _FakeServer.open();
    final client = AcpClient.channel(wire.clientChannel);
    await client.connect();
    addTearDown(client.close);

    expect(client.authMethods, isEmpty);
    await client.authenticate('opencode-login');
    expect(wire.methods, contains('authenticate'));
    await wire.close();
  });

  test(
    'closing one memory transport completes the peer incoming stream',
    () async {
      final pair = MemoryTransportPair();
      final done = pair.right.incoming.drain<void>();
      await pair.left.close();
      await done.timeout(const Duration(seconds: 1));
    },
  );
}

/// Flushes pending microtasks and stream events.
Future<void> pumpEventQueue() async {
  for (var i = 0; i < 20; i++) {
    await Future<void>.delayed(Duration.zero);
  }
}

class _Recorded {
  _Recorded(this.method, this.params);
  final String method;
  final Object? params;
}

/// A scripted ACP agent implemented with acpd's AgentRole, exercising the
/// client without a real runtime. Emits the ACP subset the client relies on
/// and can inject unknown update kinds.
final class _FakeServer {
  _FakeServer._({required this.deleteUnsupported, required this.promptDelay});

  final bool deleteUnsupported;
  final Duration promptDelay;
  final _controller = StreamChannelController<String>();
  final _requests = <_Recorded>[];
  final _cancelled = <String>[];
  final _closed = <String>[];
  final _permissionOutcomes = <acpd.RequestPermissionOutcome>[];
  late final acpd.AgentConnection _agentConn;

  static final _configOptions = <acpd.SessionConfigOption>[
    acpd.SessionConfigSelectOptionValue(
      id: 'model',
      name: 'Model',
      category: acpd.SessionConfigOptionCategory.model,
      currentValue: 'opencode/foo',
      options: acpd.SessionConfigUngroupedOptions(const [
        acpd.SessionConfigSelectOption(
          value: 'opencode/foo',
          name: 'Foo Model',
        ),
        acpd.SessionConfigSelectOption(
          value: 'opencode/bar',
          name: 'Bar Model',
        ),
      ]),
    ),
    acpd.SessionConfigSelectOptionValue(
      id: 'effort',
      name: 'Effort',
      category: acpd.SessionConfigOptionCategory.thoughtLevel,
      currentValue: 'low',
      options: acpd.SessionConfigUngroupedOptions(const [
        acpd.SessionConfigSelectOption(value: 'low', name: 'Low'),
        acpd.SessionConfigSelectOption(value: 'high', name: 'High'),
      ]),
    ),
    acpd.SessionConfigSelectOptionValue(
      id: 'mode',
      name: 'Session Mode',
      category: acpd.SessionConfigOptionCategory.mode,
      currentValue: 'build',
      options: acpd.SessionConfigUngroupedOptions(const [
        acpd.SessionConfigSelectOption(value: 'build', name: 'build'),
        acpd.SessionConfigSelectOption(value: 'plan', name: 'plan'),
      ]),
    ),
  ];

  StreamChannel<String> get clientChannel => _controller.foreign;

  /// Raw requests received in arrival order.
  List<_Recorded> get requests => _requests;

  List<String> get methods => _requests.map((r) => r.method).toList();

  /// Sessions that received a `session/cancel` notification.
  List<String> get cancelledSessions => _cancelled;

  /// Sessions that received a `session/close` request.
  List<String> get closedSessions => _closed;

  /// Outcomes observed for pushed permission requests.
  List<acpd.RequestPermissionOutcome> get permissionOutcomes =>
      _permissionOutcomes;

  static Future<_FakeServer> open({
    Duration promptDelay = Duration.zero,
    bool deleteUnsupported = false,
  }) async {
    final server = _FakeServer._(
      deleteUnsupported: deleteUnsupported,
      promptDelay: promptDelay,
    );
    final agent = acpd.AgentRole()
      ..onInitialize((context, request, cancellation) {
        server._requests.add(_Recorded('initialize', request.toJson()));
        return initializeResult();
      })
      ..onNewSession((context, request, cancellation) {
        server._requests.add(_Recorded('session/new', request.toJson()));
        return acpd.NewSessionResponse(
          sessionId: 'sess-1',
          configOptions: _configOptions,
        );
      })
      ..onSetSessionConfigOption((context, request, cancellation) {
        server._requests.add(
          _Recorded('session/set_config_option', request.toJson()),
        );
        return acpd.SetSessionConfigOptionResponse(
          configOptions: _configOptions,
        );
      })
      ..onListSessions((context, request, cancellation) {
        server._requests.add(_Recorded('session/list', request.toJson()));
        return acpd.ListSessionsResponse(sessions: const [], nextCursor: null);
      })
      ..onLoadSession((context, request, cancellation) {
        server._requests.add(_Recorded('session/load', request.toJson()));
        server._replayTimeline(context, request.sessionId);
        return acpd.LoadSessionResponse(configOptions: _configOptions);
      })
      ..onPrompt((context, request, cancellation) async {
        server._requests.add(_Recorded('session/prompt', request.toJson()));
        if (promptDelay > Duration.zero) {
          await Future<void>.delayed(promptDelay);
          if (server._cancelled.contains(request.sessionId)) {
            return acpd.PromptResponse(stopReason: acpd.StopReason.cancelled);
          }
        }
        final text = request.prompt
            .whereType<acpd.TextContentBlock>()
            .map((b) => b.text)
            .join('\n');
        if (text.trim() == '/compact') {
          server._streamCompact(context);
        } else {
          server._streamTurn(context);
        }
        return acpd.PromptResponse(stopReason: acpd.StopReason.endTurn);
      })
      ..onSessionCancel((context, params) {
        server._cancelled.add(params.sessionId);
      })
      ..onDeleteSession((context, request, cancellation) {
        server._requests.add(_Recorded('session/delete', request.toJson()));
        if (deleteUnsupported) {
          throw acpd.RpcError(code: -32601, message: 'Method not found');
        }
        return acpd.DeleteSessionResponse();
      })
      ..onCloseSession((context, request, cancellation) {
        server._requests.add(_Recorded('session/close', request.toJson()));
        server._closed.add(request.sessionId);
        return acpd.CloseSessionResponse();
      })
      ..onAuthenticate((context, request, cancellation) {
        server._requests.add(_Recorded('authenticate', request.toJson()));
        return acpd.AuthenticateResponse();
      });
    server._agentConn = agent.connect(
      ChannelTransport(server._controller.local),
    );
    return server;
  }

  Future<void> close() async {
    await _agentConn.close();
  }

  void _streamCompact(acpd.AgentContext context) {
    const sessionId = 'sess-1';
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.AgentMessageChunk(
        chunk: acpd.ContentChunk(
          messageId: 'msg-compact',
          content: const acpd.TextContentBlock(
            text: 'Context compacted. Kept 3 recent messages.',
          ),
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.UsageSessionUpdate(used: 1200, size: 8000),
    );
  }

  void _streamTurn(acpd.AgentContext context) {
    const sessionId = 'sess-1';
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.PlanUpdate(
        plan: acpd.Plan(
          entries: const [
            acpd.PlanEntry(
              content: 'Check syntax',
              priority: acpd.PlanEntryPriority.high,
              status: acpd.PlanEntryStatus.pending,
            ),
            acpd.PlanEntry(
              content: 'Fix issues',
              priority: acpd.PlanEntryPriority.medium,
              status: acpd.PlanEntryStatus.inProgress,
            ),
          ],
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.AgentThoughtChunk(
        chunk: acpd.ContentChunk(
          messageId: 'msg-1',
          content: const acpd.TextContentBlock(text: 'thinking'),
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.AgentMessageChunk(
        chunk: acpd.ContentChunk(
          messageId: 'msg-2',
          content: const acpd.TextContentBlock(text: 'answer'),
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.ToolCallUpdateSession(
        toolCall: acpd.ToolCall(
          toolCallId: 'call-1',
          title: 'Read: .',
          kind: acpd.ToolKind.read,
          status: acpd.ToolCallStatus.pending,
          rawInput: {'path': '.'},
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.ToolCallStatusUpdate(
        update: acpd.ToolCallUpdate(
          toolCallId: 'call-1',
          status: acpd.ToolCallStatus.completed,
          rawOutput: {'output': 'file list'},
        ),
      ),
    );
  }

  void _replayTimeline(acpd.AgentContext context, String sessionId) {
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.UserMessageChunk(
        chunk: acpd.ContentChunk(
          messageId: 'msg-user',
          content: const acpd.TextContentBlock(text: 'hi'),
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.AgentMessageChunk(
        chunk: acpd.ContentChunk(
          messageId: 'msg-assistant',
          content: const acpd.TextContentBlock(text: 'answer'),
        ),
      ),
    );
    context.sessionUpdate(
      sessionId: sessionId,
      update: acpd.ToolCallUpdateSession(
        toolCall: acpd.ToolCall(
          toolCallId: 'call-1',
          title: 'Read: .',
          kind: acpd.ToolKind.read,
          status: acpd.ToolCallStatus.pending,
          rawInput: {'path': '.'},
        ),
      ),
    );
  }

  void pushUnknownUpdate() {
    _controller.local.sink.add(
      jsonEncode({
        'jsonrpc': '2.0',
        'method': 'session/update',
        'params': {
          'sessionId': 'sess-1',
          'update': {
            'sessionUpdate': 'mystery_update',
            'payload': <String, Object?>{},
          },
        },
      }),
    );
  }

  /// Sends a notification through the agent context.
  void push(acpd.SessionUpdate update) =>
      _agentConn.agent.sessionUpdate(sessionId: 'sess-1', update: update);

  void pushInfoCommandsAndMode() {
    push(acpd.SessionInfoSessionUpdate(title: 'My session'));
    push(
      acpd.AvailableCommandsSessionUpdate(
        update: acpd.AvailableCommandsUpdate(
          availableCommands: const [
            acpd.AvailableCommand(name: 'web', description: 'Search the web'),
          ],
        ),
      ),
    );
    push(acpd.CurrentModeSessionUpdate(currentModeId: 'build'));
  }

  void pushPermissionRequest() {
    _agentConn.agent
        .requestPermission(
          acpd.RequestPermissionRequest(
            sessionId: 'sess-1',
            toolCall: const acpd.ToolCallUpdate(
              toolCallId: 'call-9',
              title: 'Edit: /tmp/a.txt',
              kind: acpd.ToolKind.edit,
              status: acpd.ToolCallStatus.pending,
            ),
            options: const [
              acpd.PermissionOption(
                optionId: 'once',
                name: 'Allow once',
                kind: acpd.PermissionOptionKind.allowOnce,
              ),
              acpd.PermissionOption(
                optionId: 'always',
                name: 'Always allow',
                kind: acpd.PermissionOptionKind.allowAlways,
              ),
              acpd.PermissionOption(
                optionId: 'reject',
                name: 'Reject',
                kind: acpd.PermissionOptionKind.rejectOnce,
              ),
            ],
          ),
        )
        .then((outcome) {
          _permissionOutcomes.add(outcome);
        })
        .catchError((Object _) {});
  }
}

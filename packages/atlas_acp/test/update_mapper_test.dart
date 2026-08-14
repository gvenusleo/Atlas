import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  final session = SessionId('s1');
  final turn = TurnId('t1');
  final time = DateTime.utc(2026, 1, 1);

  group('planEntries', () {
    test('converts a valid plan argument list', () {
      final entries = planEntries([
        {'step': 'Read files', 'status': 'pending'},
        {'step': 'Fix bug', 'status': 'in_progress'},
      ]);
      expect(entries, [
        {'content': 'Read files', 'priority': 'medium', 'status': 'pending'},
        {'content': 'Fix bug', 'priority': 'medium', 'status': 'in_progress'},
      ]);
    });

    test('trims step text', () {
      final entries = planEntries([
        {'step': '  spaced  ', 'status': 'completed'},
      ]);
      expect(entries!.single['content'], 'spaced');
    });

    test('rejects non-list, empty, or malformed payloads', () {
      expect(planEntries(null), isNull);
      expect(planEntries('plan'), isNull);
      expect(planEntries(const []), isNull);
      expect(
        planEntries([
          {'step': 'A'},
        ]),
        isNull,
      );
      expect(
        planEntries([
          {'step': 'A', 'status': 'queued'},
        ]),
        isNull,
      );
      expect(
        planEntries([
          {'step': '', 'status': 'pending'},
        ]),
        isNull,
      );
      expect(planEntries(['not a map']), isNull);
    });
  });

  group('toolCallKind', () {
    test('maps known tools and defaults to other', () {
      expect(toolCallKind('read'), 'read');
      expect(toolCallKind('write'), 'edit');
      expect(toolCallKind('edit'), 'edit');
      expect(toolCallKind('shell'), 'execute');
      expect(toolCallKind('plan'), 'think');
      expect(toolCallKind('anything'), 'other');
    });
  });

  group('TurnUpdateMapper', () {
    test('shares one message id across assistant chunks', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper
          .map(
            ModelTextDelta(
              sessionId: session,
              turnId: turn,
              sequence: 0,
              occurredAt: time,
              delta: 'Hello',
            ),
          )
          .followedBy(
            mapper.map(
              ModelTextDelta(
                sessionId: session,
                turnId: turn,
                sequence: 1,
                occurredAt: time,
                delta: ' world',
              ),
            ),
          )
          .toList();
      expect(updates, hasLength(2));
      expect(updates[0].update['sessionUpdate'], 'agent_message_chunk');
      expect(updates[0].update['messageId'], updates[1].update['messageId']);
    });

    test('uses distinct message ids across assistant messages in one turn', () {
      final mapper = TurnUpdateMapper(session);
      final all = <SessionUpdate>[
        ...mapper.map(
          ModelTextDelta(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            delta: 'First',
          ),
        ),
        ...mapper.map(
          ModelResponseReceived(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
            assistantMessage: _assistant([TextContent('I will check.')]),
            toolCalls: [_toolCallItem('call-1', 'read', 2)],
          ),
        ),
        ...mapper.map(
          ModelTextDelta(
            sessionId: session,
            turnId: turn,
            sequence: 3,
            occurredAt: time,
            delta: 'Second',
          ),
        ),
      ];
      final chunks = all
          .where(
            (update) => update.update['sessionUpdate'] == 'agent_message_chunk',
          )
          .toList();
      expect(chunks, hasLength(2));
      expect(
        chunks[0].update['messageId'],
        isNot(chunks[1].update['messageId']),
      );
    });

    test('tool_call titles are short human-readable phrases', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelResponseReceived(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          assistantMessage: _assistant([TextContent('I will check.')]),
          toolCalls: [_toolCallItem('call-1', 'read', 1)],
        ),
      );
      expect(updates.single.update['title'], 'Read file');
    });

    test('tool_call titles fall back to the tool name for unknown tools', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelResponseReceived(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          assistantMessage: _assistant([TextContent('I will check.')]),
          toolCalls: [_toolCallItem('call-1', 'search', 1)],
        ),
      );
      expect(updates.single.update['title'], 'search');
    });

    test('tool_call includes the raw input arguments', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelResponseReceived(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          assistantMessage: _assistant([TextContent('I will check.')]),
          toolCalls: [
            _toolCallItem('call-1', 'read', 1, arguments: {'path': '/tmp/a'}),
          ],
        ),
      );
      expect(updates.single.update['rawInput'], {'path': '/tmp/a'});
    });

    test('tool_call reports file locations for path-based tools', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelResponseReceived(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          assistantMessage: _assistant([TextContent('I will check.')]),
          toolCalls: [
            _toolCallItem(
              'call-1',
              'edit',
              1,
              arguments: {'path': '/tmp/main.dart'},
            ),
          ],
        ),
      );
      expect(updates.single.update['locations'], [
        {'path': '/tmp/main.dart'},
      ]);
    });

    test('tool_call omits locations when the tool has no file path', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelResponseReceived(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          assistantMessage: _assistant([TextContent('I will check.')]),
          toolCalls: [
            _toolCallItem('call-1', 'shell', 1, arguments: {'command': 'ls'}),
          ],
        ),
      );
      expect(updates.single.update.containsKey('locations'), isFalse);
    });

    test('completed tool results include the raw output', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolFinished(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          result: _resultItem('call-1', 'file list', 1),
        ),
      );
      expect(updates.single.update['rawOutput'], {'output': 'file list'});
    });

    test('failed tool results omit the raw output', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolFinished(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          result: _resultItem('call-1', 'boom', 1, isError: true),
        ),
      );
      expect(updates.single.update.containsKey('rawOutput'), isFalse);
    });

    test('maps a tool loop to pending, in_progress, and completed updates', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
          ModelResponseReceived(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            assistantMessage: _assistant([TextContent('I will check.')]),
            toolCalls: [_toolCallItem('call-1', 'read', 1)],
          ),
        ),
        ...mapper.map(
          ToolStarted(
            sessionId: session,
            turnId: turn,
            sequence: 2,
            occurredAt: time,
            call: _toolCallItem('call-1', 'read', 1),
          ),
        ),
        ...mapper.map(
          ToolFinished(
            sessionId: session,
            turnId: turn,
            sequence: 3,
            occurredAt: time,
            result: _resultItem('call-1', 'file list', 4),
          ),
        ),
      ];

      expect(updates, hasLength(3));
      expect(updates[0].update['sessionUpdate'], 'tool_call');
      expect(updates[0].update['toolCallId'], 'call-1');
      expect(updates[0].update['kind'], 'read');
      expect(updates[0].update['status'], 'pending');
      expect(updates[1].update['sessionUpdate'], 'tool_call_update');
      expect(updates[1].update['status'], 'in_progress');
      expect(updates[2].update['sessionUpdate'], 'tool_call_update');
      expect(updates[2].update['status'], 'completed');
    });

    test('reports plan tool calls as plan updates without tool updates', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
          ToolStarted(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            call: _toolCallItem(
              'call-plan',
              'plan',
              0,
              arguments: {
                'plan': [
                  {'step': 'A', 'status': 'pending'},
                  {'step': 'B', 'status': 'in_progress'},
                ],
              },
            ),
          ),
        ),
        ...mapper.map(
          ToolFinished(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
            result: _resultItem('call-plan', 'Plan updated', 2),
          ),
        ),
      ];

      expect(updates, hasLength(1));
      expect(updates.single.update['sessionUpdate'], 'plan');
      final entries = updates.single.update['entries'] as List;
      expect(entries, hasLength(2));
    });

    test('marks failed tool results with status failed', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolFinished(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          result: _resultItem('call-1', 'boom', 1, isError: true),
        ),
      );
      expect(updates.single.update['status'], 'failed');
    });

    test('ignores turn lifecycle and compaction events', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
          TurnStarted(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            userMessage: _user('hello'),
          ),
        ),
        ...mapper.map(
          CompactionStarted(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
          ),
        ),
        ...mapper.map(
          TurnFinished(
            sessionId: session,
            turnId: turn,
            sequence: 2,
            occurredAt: time,
            outcome: TurnOutcome(
              sessionId: session,
              turnId: turn,
              status: TurnStatus.completed,
            ),
          ),
        ),
      ];
      expect(updates, isEmpty);
    });
  });

  group('replayTimeline', () {
    test('replays messages, tool calls, and results in order', () {
      final timeline = <TimelineItem>[
        _user('first', id: 'item-1'),
        _assistantItem([TextContent('reply')], id: 'item-2'),
        _toolCallItem('call-1', 'read', 3, id: 'item-3'),
        _resultItem('call-1', 'file list', 4, id: 'item-4'),
      ];
      final updates = replayTimeline(timeline);
      expect(updates.map((u) => u.update['sessionUpdate']), [
        'user_message_chunk',
        'agent_message_chunk',
        'tool_call',
        'tool_call_update',
      ]);
      expect(updates[0].update['messageId'], 'item-1');
      expect(updates[2].update['toolCallId'], 'call-1');
      expect(updates[2].update['rawInput'], <String, Object?>{});
      expect(updates[3].update['status'], 'completed');
      expect(updates[3].update['rawOutput'], {'output': 'file list'});
    });

    test('replays plan tool calls as plan updates', () {
      final timeline = <TimelineItem>[
        _toolCallItem(
          'call-plan',
          'plan',
          0,
          id: 'item-1',
          arguments: {
            'plan': [
              {'step': 'A', 'status': 'completed'},
            ],
          },
        ),
        _resultItem('call-plan', 'Plan updated', 1, id: 'item-2'),
      ];
      final updates = replayTimeline(timeline);
      expect(updates, hasLength(1));
      expect(updates.single.update['sessionUpdate'], 'plan');
    });

    test('replay uses short human-readable tool_call titles', () {
      final timeline = <TimelineItem>[
        _toolCallItem('call-1', 'read', 0, id: 'item-1'),
      ];
      final updates = replayTimeline(timeline);
      expect(updates.single.update['title'], 'Read file');
    });

    test('replay reports locations for path-based tool calls', () {
      final timeline = <TimelineItem>[
        _toolCallItem(
          'call-1',
          'write',
          0,
          id: 'item-1',
          arguments: {'path': '/tmp/out.txt', 'content': 'hi'},
        ),
      ];
      final updates = replayTimeline(timeline);
      expect(updates.single.update['locations'], [
        {'path': '/tmp/out.txt'},
      ]);
    });

    test('replay skips only the plan result matching its own call', () {
      // The model reuses the call id across turns: turn one is a plan call,
      // turn two is a plain read with the same id. Only the plan result is
      // skipped.
      final timeline = <TimelineItem>[
        UserMessageItem(
          id: TimelineItemId('item-1'),
          sessionId: session,
          turnId: TurnId('t1'),
          sequence: 0,
          occurredAt: time,
          content: const [TextContent('plan it')],
        ),
        _toolCallItem(
          'call-1',
          'plan',
          1,
          id: 'item-2',
          arguments: {
            'plan': [
              {'step': 'A', 'status': 'completed'},
            ],
          },
        ),
        _resultItem('call-1', 'Plan updated', 2, id: 'item-3'),
        UserMessageItem(
          id: TimelineItemId('item-4'),
          sessionId: session,
          turnId: TurnId('t2'),
          sequence: 3,
          occurredAt: time,
          content: const [TextContent('read it')],
        ),
        _toolCallItem('call-1', 'read', 4, id: 'item-5'),
        _resultItem('call-1', 'file list', 5, id: 'item-6'),
      ];
      final updates = replayTimeline(timeline);
      expect(updates.map((u) => u.update['sessionUpdate']), [
        'user_message_chunk',
        'plan',
        'user_message_chunk',
        'tool_call',
        'tool_call_update',
      ]);
      expect(updates.last.update['status'], 'completed');
    });
  });
}

AssistantMessageItem _assistantItem(
  List<ContentPart> content, {
  required String id,
}) => AssistantMessageItem(
  id: TimelineItemId(id),
  sessionId: SessionId('s1'),
  turnId: TurnId('t1'),
  sequence: 0,
  occurredAt: DateTime.utc(2026, 1, 1),
  content: content,
  model: ModelRef(providerId: ProviderId('p'), modelId: ModelId('m')),
  stopReason: StopReason.endTurn,
);

AssistantMessageItem _assistant(List<ContentPart> content) =>
    _assistantItem(content, id: 'assistant-1');

UserMessageItem _user(String text, {String id = 'user-1'}) => UserMessageItem(
  id: TimelineItemId(id),
  sessionId: SessionId('s1'),
  turnId: TurnId('t1'),
  sequence: 0,
  occurredAt: DateTime.utc(2026, 1, 1),
  content: [TextContent(text)],
);

ToolCallItem _toolCallItem(
  String callId,
  String name,
  int sequence, {
  String id = 'call-item',
  Map<String, Object?> arguments = const {},
}) => ToolCallItem(
  id: TimelineItemId(id),
  sessionId: SessionId('s1'),
  turnId: TurnId('t1'),
  sequence: sequence,
  occurredAt: DateTime.utc(2026, 1, 1),
  call: ToolCall(id: ToolCallId(callId), name: name, arguments: arguments),
);

ToolResultItem _resultItem(
  String callId,
  String content,
  int sequence, {
  String id = 'result-item',
  bool isError = false,
}) => ToolResultItem(
  id: TimelineItemId(id),
  sessionId: SessionId('s1'),
  turnId: TurnId('t1'),
  sequence: sequence,
  occurredAt: DateTime.utc(2026, 1, 1),
  callId: ToolCallId(callId),
  content: content,
  isError: isError,
);

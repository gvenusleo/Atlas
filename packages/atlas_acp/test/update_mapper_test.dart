import 'package:acpd/acpd.dart' hide StopReason, TextContent, ToolCall;
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
    });
  });

  group('TurnUpdateMapper', () {
    test('assistant text deltas become agent message chunks', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelTextDelta(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          delta: 'Hello',
        ),
      );
      final chunk = updates.single as AgentMessageChunk;
      expect((chunk.chunk.content as TextContentBlock).text, 'Hello');
    });

    test('reasoning deltas become agent thought chunks', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ModelReasoningDelta(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          delta: 'thinking',
        ),
      );
      final chunk = updates.single as AgentThoughtChunk;
      expect((chunk.chunk.content as TextContentBlock).text, 'thinking');
    });

    test('chunks of one message share a message id', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
          ModelTextDelta(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            delta: 'Hello',
          ),
        ),
        ...mapper.map(
          ModelTextDelta(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
            delta: ' world',
          ),
        ),
      ];
      expect(updates, hasLength(2));
      expect(
        (updates[0] as AgentMessageChunk).chunk.messageId,
        (updates[1] as AgentMessageChunk).chunk.messageId,
      );
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
      final chunks = all.whereType<AgentMessageChunk>().toList();
      expect(chunks, hasLength(2));
      expect(chunks[0].chunk.messageId, isNot(chunks[1].chunk.messageId));
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
      final toolCall = (updates.single as ToolCallUpdateSession).toolCall;
      expect(toolCall.title, 'Read file');
      expect(toolCall.kind, ToolKind.read);
    });

    test('shell tool calls use the command as their title', () {
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
              'shell',
              1,
              arguments: {'command': 'ls -la'},
            ),
          ],
        ),
      );
      final toolCall = (updates.single as ToolCallUpdateSession).toolCall;
      expect(toolCall.title, 'ls -la');
    });

    test('shell tool calls embed a display-only terminal reference', () {
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
              'shell',
              1,
              arguments: {'command': 'ls -la', 'cwd': '/tmp'},
            ),
          ],
        ),
      );
      final toolCall = (updates.single as ToolCallUpdateSession).toolCall;
      expect(
        toolCall.content.whereType<ToolCallTerminal>().single.terminalId,
        'term-call-1',
      );
      expect(
        (toolCall.meta['terminal_info'] as Map<String, Object?>)['terminal_id'],
        'term-call-1',
      );
    });

    test('tool_call titles truncate shell commands over the limit', () {
      final mapper = TurnUpdateMapper(session);
      final longCommand = 'echo ${'x' * 2000}';
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
              'shell',
              1,
              arguments: {'command': longCommand},
            ),
          ],
        ),
      );
      final title = (updates.single as ToolCallUpdateSession).toolCall.title;
      expect(title, 'echo ${'x' * 995}…');
      expect(title.codeUnits.length, shellTitleLimit + 1);
    });

    test('in_progress shell updates keep the terminal reference', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolStarted(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          call: _toolCallItem(
            'call-1',
            'shell',
            1,
            arguments: {'command': 'ls -la'},
          ),
        ),
      );
      final update = (updates.single as ToolCallStatusUpdate).update;
      expect(update.status, ToolCallStatus.inProgress);
      expect(
        update.content!.whereType<ToolCallTerminal>().single.terminalId,
        'term-call-1',
      );
    });

    test('file tool results render as diffs with follow-along locations', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
          ModelResponseReceived(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            assistantMessage: _assistant([TextContent('I will check.')]),
            toolCalls: [_toolCallItem('call-1', 'edit', 1)],
          ),
        ),
        ...mapper.map(
          ToolFinished(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
            result: _resultItem(
              'call-1',
              'Applied 1 edit(s) to /tmp/a.dart',
              2,
              metadata: const {
                'path': '/tmp/a.dart',
                'oldText': 'final a = 1;\n',
                'newText': 'final a = 2;\n',
                'line': 1,
              },
            ),
          ),
        ),
      ];
      final update = (updates.last as ToolCallStatusUpdate).update;
      expect(update.status, ToolCallStatus.completed);
      final diff = update.content!.whereType<ToolCallDiff>().single;
      expect(diff.path, '/tmp/a.dart');
      expect(diff.oldText, 'final a = 1;\n');
      expect(diff.newText, 'final a = 2;\n');
      expect(update.locations!.single.line, 1);
    });

    test('new file writes render as diffs with no old text', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
          ModelResponseReceived(
            sessionId: session,
            turnId: turn,
            sequence: 0,
            occurredAt: time,
            assistantMessage: _assistant([TextContent('I will check.')]),
            toolCalls: [_toolCallItem('call-1', 'write', 1)],
          ),
        ),
        ...mapper.map(
          ToolFinished(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
            result: _resultItem(
              'call-1',
              'Wrote 5 bytes to /tmp/new.dart',
              2,
              metadata: const {'path': '/tmp/new.dart', 'newText': 'hello'},
            ),
          ),
        ),
      ];
      final update = (updates.last as ToolCallStatusUpdate).update;
      final diff = update.content!.whereType<ToolCallDiff>().single;
      expect(diff.path, '/tmp/new.dart');
      expect(diff.oldText, isNull);
      expect(diff.newText, 'hello');
      expect(update.locations!.single.line, isNull);
    });

    test('failed file results keep the text summary instead of a diff', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolFinished(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          result: _resultItem(
            'call-1',
            'old_text not found: x',
            1,
            isError: true,
            metadata: const {'path': '/tmp/a.dart', 'newText': 'changed'},
          ),
        ),
      );
      final update = (updates.single as ToolCallStatusUpdate).update;
      expect(update.status, ToolCallStatus.failed);
      final content = update.content!
          .whereType<ToolCallContentBlock>()
          .single
          .content;
      expect((content as TextContentBlock).text, 'old_text not found: x');
      expect(update.locations, isNull);
    });

    test('non-file tools never render diffs from stray metadata', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolFinished(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          result: _resultItem(
            'call-1',
            'search results',
            1,
            metadata: const {'path': '/tmp/a.dart', 'newText': 'changed'},
          ),
        ),
      );
      final update = (updates.single as ToolCallStatusUpdate).update;
      final content = update.content!
          .whereType<ToolCallContentBlock>()
          .single
          .content;
      expect((content as TextContentBlock).text, 'search results');
    });

    test('completed shell results stream output through terminal meta', () {
      final mapper = TurnUpdateMapper(session);
      final updates = <SessionUpdate>[
        ...mapper.map(
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
        ),
        ...mapper.map(
          ToolFinished(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: time,
            result: _resultItem(
              'call-1',
              'file list',
              2,
              metadata: const {'exit_code': 1},
            ),
          ),
        ),
      ];
      final update = (updates.last as ToolCallStatusUpdate).update;
      expect(update.status, ToolCallStatus.completed);
      expect(
        update.content!.whereType<ToolCallTerminal>().single.terminalId,
        'term-call-1',
      );
      expect(
        (update.meta['terminal_exit'] as Map<String, Object?>)['exit_code'],
        1,
      );
    });

    test(
      'plan tool calls become plan updates and their results are dropped',
      () {
        final mapper = TurnUpdateMapper(session);
        final updates = <SessionUpdate>[
          ...mapper.map(
            ToolStarted(
              sessionId: session,
              turnId: turn,
              sequence: 0,
              occurredAt: time,
              call: _toolCallItem(
                'call-1',
                'plan',
                0,
                arguments: {
                  'plan': [
                    {'step': 'A', 'status': 'completed'},
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
              result: _resultItem('call-1', 'Plan updated', 1),
            ),
          ),
        ];
        expect(updates, hasLength(1));
        final plan = (updates.single as PlanUpdate).plan;
        expect(plan.entries.single.content, 'A');
        expect(plan.entries.single.status, PlanEntryStatus.completed);
      },
    );
  });

  group('replayTimeline', () {
    test('replays user, assistant, thought, tool, and result items', () {
      final timeline = <TimelineItem>[
        _user('hello'),
        _assistantItem(const [TextContent('answer')], id: 'a-1'),
        _toolCallItem('call-1', 'read', 1, id: 'item-2'),
        _resultItem('call-1', 'file list', 2, id: 'item-3'),
      ];
      final updates = replayTimeline(timeline);
      expect(updates, hasLength(4));
      expect(updates[0], isA<UserMessageChunk>());
      expect(updates[1], isA<AgentMessageChunk>());
      expect(updates[2], isA<ToolCallUpdateSession>());
      expect(updates[3], isA<ToolCallStatusUpdate>());
    });

    test('replays reasoning as thought chunks before the message', () {
      final timeline = <TimelineItem>[
        _assistantItem(
          const [TextContent('answer')],
          id: 'a-1',
          reasoning: 'thinking',
        ),
      ];
      final updates = replayTimeline(timeline);
      expect(updates[0], isA<AgentThoughtChunk>());
      expect(updates[1], isA<AgentMessageChunk>());
    });

    test('resolves relative tool paths against the working directory', () {
      final timeline = <TimelineItem>[
        _toolCallItem(
          'call-1',
          'read',
          1,
          arguments: {'path': 'lib/main.dart', 'offset': 3},
        ),
      ];
      final updates = replayTimeline(timeline, workingDirectory: '/project');
      final toolCall = (updates.single as ToolCallUpdateSession).toolCall;
      expect(toolCall.locations.single.path, '/project/lib/main.dart');
      expect(toolCall.locations.single.line, 3);
    });

    test('plan items replay as plan updates and skip results', () {
      final timeline = <TimelineItem>[
        _assistantItem(const [TextContent('plan it')], id: 'item-1'),
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
      expect(updates, hasLength(5));
      expect(updates[0], isA<AgentMessageChunk>());
      expect(updates[1], isA<PlanUpdate>());
      expect(updates[2], isA<UserMessageChunk>());
      expect(updates[3], isA<ToolCallUpdateSession>());
      expect(updates[4], isA<ToolCallStatusUpdate>());
    });
  });
}

AssistantMessageItem _assistantItem(
  List<ContentPart> content, {
  required String id,
  String reasoning = '',
}) => AssistantMessageItem(
  id: TimelineItemId(id),
  sessionId: SessionId('s1'),
  turnId: TurnId('t1'),
  sequence: 0,
  occurredAt: DateTime.utc(2026, 1, 1),
  content: content,
  model: ModelRef(providerId: ProviderId('p'), modelId: ModelId('m')),
  stopReason: StopReason.endTurn,
  reasoning: reasoning,
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
  Map<String, Object?> metadata = const {},
}) => ToolResultItem(
  id: TimelineItemId(id),
  sessionId: SessionId('s1'),
  turnId: TurnId('t1'),
  sequence: sequence,
  occurredAt: DateTime.utc(2026, 1, 1),
  callId: ToolCallId(callId),
  content: content,
  isError: isError,
  metadata: metadata,
);

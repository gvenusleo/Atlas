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

  group('toolCallLocations', () {
    test('reports paths and the read start line', () {
      expect(toolCallLocations('read', {'path': 'a.dart'}), [
        {'path': 'a.dart'},
      ]);
      expect(toolCallLocations('read', {'path': 'a.dart', 'offset': 42}), [
        {'path': 'a.dart', 'line': 42},
      ]);
      expect(toolCallLocations('write', {'path': 'a.dart'}), [
        {'path': 'a.dart'},
      ]);
      expect(toolCallLocations('shell', {'command': 'ls'}), isEmpty);
    });

    test('resolves relative paths against the working directory', () {
      expect(
        toolCallLocations('read', {
          'path': 'src/a.dart',
          'offset': 3,
        }, workingDirectory: '/home/user/project'),
        [
          {'path': '/home/user/project/src/a.dart', 'line': 3},
        ],
      );
      expect(
        toolCallLocations('edit', {
          'path': '/abs/path/b.dart',
        }, workingDirectory: '/home/user/project'),
        [
          {'path': '/abs/path/b.dart'},
        ],
      );
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

    test('tool_call titles include the target path when present', () {
      final mapper = TurnUpdateMapper(session);
      List<SessionUpdate> titles(String name, Map<String, Object?> arguments) =>
          mapper.map(
            ModelResponseReceived(
              sessionId: session,
              turnId: turn,
              sequence: 0,
              occurredAt: time,
              assistantMessage: _assistant([TextContent('I will check.')]),
              toolCalls: [
                _toolCallItem('call-1', name, 1, arguments: arguments),
              ],
            ),
          );

      expect(
        titles('read', {'path': '/tmp/a.txt'}).single.update['title'],
        'Read: /tmp/a.txt',
      );
      expect(
        titles('write', {'path': 'lib/a.dart'}).single.update['title'],
        'Write: lib/a.dart',
      );
      expect(
        titles('edit', {'path': 'test/a_test.dart'}).single.update['title'],
        'Edit: test/a_test.dart',
      );
    });

    test('tool_call title for plan shows completed over total steps', () {
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
              'plan',
              1,
              arguments: {
                'plan': [
                  {'step': 'Read files', 'status': 'completed'},
                  {'step': 'Fix bug', 'status': 'in_progress'},
                  {'step': 'Verify', 'status': 'pending'},
                ],
              },
            ),
          ],
        ),
      );
      expect(updates.single.update['title'], 'Plan: 1/3 completed');
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

    test('tool_call titles show the shell command being run', () {
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
      expect(updates.single.update['title'], 'ls -la');
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
      final update = updates.single.update;
      expect(update['content'], [
        {'type': 'terminal', 'terminalId': 'term-call-1'},
      ]);
      expect(update['_meta'], {
        'terminal_info': {'terminal_id': 'term-call-1', 'cwd': '/tmp'},
      });
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
      final title = updates.single.update['title'] as String;
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
      expect(updates.single.update['status'], 'in_progress');
      expect(updates.single.update['content'], [
        {'type': 'terminal', 'terminalId': 'term-call-1'},
      ]);
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
      final completed = updates.last.update;
      expect(completed['status'], 'completed');
      expect(completed['content'], [
        {
          'type': 'diff',
          'path': '/tmp/a.dart',
          'oldText': 'final a = 1;\n',
          'newText': 'final a = 2;\n',
        },
      ]);
      expect(completed['locations'], [
        {'path': '/tmp/a.dart', 'line': 1},
      ]);
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
      final completed = updates.last.update;
      expect(completed['content'], [
        {
          'type': 'diff',
          'path': '/tmp/new.dart',
          'oldText': null,
          'newText': 'hello',
        },
      ]);
      expect(completed['locations'], [
        {'path': '/tmp/new.dart'},
      ]);
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
      final completed = updates.single.update;
      expect(completed['status'], 'failed');
      expect(completed['content'], [
        {
          'type': 'content',
          'content': {'type': 'text', 'text': 'old_text not found: x'},
        },
      ]);
      expect(completed.containsKey('locations'), isFalse);
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
      expect(updates.single.update['content'], [
        {
          'type': 'content',
          'content': {'type': 'text', 'text': 'search results'},
        },
      ]);
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
              metadata: const {'exit_code': 0},
            ),
          ),
        ),
      ];
      final completed = updates.last.update;
      expect(completed['status'], 'completed');
      expect(completed['content'], [
        {'type': 'terminal', 'terminalId': 'term-call-1'},
      ]);
      expect(completed['_meta'], {
        'terminal_output': {'terminal_id': 'term-call-1', 'data': 'file list'},
        'terminal_exit': {'terminal_id': 'term-call-1', 'exit_code': 0},
      });
    });

    test('failed shell results omit the exit code from terminal meta', () {
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
            result: _resultItem('call-1', 'timed out', 2, isError: true),
          ),
        ),
      ];
      final failed = updates.last.update;
      expect(failed['status'], 'failed');
      expect(failed['_meta'], {
        'terminal_output': {'terminal_id': 'term-call-1', 'data': 'timed out'},
        'terminal_exit': {'terminal_id': 'term-call-1'},
      });
    });

    test('in_progress updates omit content for non-shell tools', () {
      final mapper = TurnUpdateMapper(session);
      final updates = mapper.map(
        ToolStarted(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: time,
          call: _toolCallItem('call-1', 'read', 1),
        ),
      );
      expect(updates.single.update['status'], 'in_progress');
      expect(updates.single.update.containsKey('content'), isFalse);
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

    test('replays assistant reasoning as thought chunks', () {
      final timeline = <TimelineItem>[
        _assistantItem(
          [TextContent('reply')],
          id: 'item-2',
          reasoning: 'thinking text',
        ),
      ];
      final updates = replayTimeline(timeline);
      expect(updates.map((u) => u.update['sessionUpdate']), [
        'agent_thought_chunk',
        'agent_message_chunk',
      ]);
      expect(updates[0].update['messageId'], 'item-2');
      expect(updates[0].update['content'], {
        'type': 'text',
        'text': 'thinking text',
      });
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

    test('replays file tool results as diffs with absolute locations', () {
      final timeline = <TimelineItem>[
        _toolCallItem(
          'call-1',
          'write',
          0,
          id: 'item-1',
          arguments: {'path': 'new.txt', 'content': 'hello'},
        ),
        _resultItem(
          'call-1',
          'Wrote 5 bytes to /tmp/project/new.txt',
          1,
          id: 'item-2',
          metadata: const {'path': '/tmp/project/new.txt', 'newText': 'hello'},
        ),
      ];
      final updates = replayTimeline(
        timeline,
        workingDirectory: '/tmp/project',
      );
      expect(updates[0].update['locations'], [
        {'path': '/tmp/project/new.txt'},
      ]);
      expect(updates[1].update['content'], [
        {
          'type': 'diff',
          'path': '/tmp/project/new.txt',
          'oldText': null,
          'newText': 'hello',
        },
      ]);
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

    test('replay keeps the terminal reference for shell calls', () {
      final timeline = <TimelineItem>[
        _toolCallItem(
          'call-1',
          'shell',
          0,
          id: 'item-1',
          arguments: {'command': 'ls'},
        ),
        _resultItem(
          'call-1',
          'file list',
          1,
          id: 'item-2',
          metadata: const {'exit_code': 0},
        ),
      ];
      final updates = replayTimeline(timeline);
      expect(updates.map((u) => u.update['sessionUpdate']), [
        'tool_call',
        'tool_call_update',
      ]);
      expect(updates[0].update['content'], [
        {'type': 'terminal', 'terminalId': 'term-call-1'},
      ]);
      expect(updates[1].update['_meta'], {
        'terminal_output': {'terminal_id': 'term-call-1', 'data': 'file list'},
        'terminal_exit': {'terminal_id': 'term-call-1', 'exit_code': 0},
      });
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

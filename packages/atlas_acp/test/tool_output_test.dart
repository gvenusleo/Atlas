import 'package:acpd/acpd.dart' as acp;
import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart' as rt;
import 'package:test/test.dart';

void main() {
  final session = rt.SessionId('s');
  final turn = rt.TurnId('t');
  final now = DateTime.utc(2026);
  final call = rt.ToolCallItem(
    id: rt.TimelineItemId('call-item'),
    sessionId: session,
    turnId: turn,
    sequence: 0,
    occurredAt: now,
    call: rt.ToolCall(
      id: rt.ToolCallId('call'),
      name: 'shell',
      arguments: const {'command': 'test'},
    ),
  );
  test(
    'round-trips in-progress snapshots and failed results through ACP JSON',
    () {
      final server = TurnUpdateMapper(session);
      final client = ClientUpdateMapper(session, turn);
      final events = <rt.AgentEvent>[];
      final source = <rt.AgentEvent>[
        rt.ToolStarted(
          sessionId: session,
          turnId: turn,
          sequence: 0,
          occurredAt: now,
          call: call,
        ),
        for (final text in ['first', 'first\nsecond'])
          rt.ToolOutputUpdated(
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: now,
            callId: call.call.id,
            output: rt.ToolOutputSnapshot(
              content: text,
              totalBytes: 123,
              truncated: true,
            ),
          ),
        rt.ToolFinished(
          sessionId: session,
          turnId: turn,
          sequence: 3,
          occurredAt: now,
          result: rt.ToolResultItem(
            id: rt.TimelineItemId('result'),
            sessionId: session,
            turnId: turn,
            sequence: 1,
            occurredAt: now,
            callId: call.call.id,
            content: 'command timed out\nfirst\nsecond',
            isError: true,
            metadata: const {
              'termination_reason': 'timed_out',
              'truncated': true,
              'total_bytes': 123,
            },
          ),
        ),
      ];
      final timeline = ClientTimelineMapper(session);
      final persisted = <rt.TimelineItem>[];
      for (final event in source) {
        for (final update in server.map(event)) {
          final decoded = acp.SessionUpdate.fromJson(update.toJson());
          final output = (decoded as acp.ToolCallStatusUpdate).update;
          if (output.content != null) {
            // Shell calls keep the display terminal reference so clients keep
            // rendering them as terminals; the text travels in rawOutput.
            expect(output.content, hasLength(1));
            expect(
              output.content!.whereType<acp.ToolCallTerminal>(),
              hasLength(1),
            );
          }
          events.addAll(client.map(decoded));
          persisted.addAll(timeline.map(decoded));
        }
      }
      expect(events.map((e) => e.runtimeType), [
        rt.ToolStarted,
        rt.ToolOutputUpdated,
        rt.ToolOutputUpdated,
        rt.ToolFinished,
      ]);
      expect(
        events.whereType<rt.ToolOutputUpdated>().last.output.content,
        'first\nsecond',
      );
      expect(
        events.whereType<rt.ToolOutputUpdated>().last.output.totalBytes,
        123,
      );
      final finished = events.whereType<rt.ToolFinished>().single.result;
      expect(finished.isError, isTrue);
      expect(finished.content, 'command timed out\nfirst\nsecond');
      expect(finished.metadata['termination_reason'], 'timed_out');
      expect(persisted.whereType<rt.ToolResultItem>(), hasLength(1));
    },
  );

  for (final clear in [false, true]) {
    test(
      'partial ACP updates ${clear ? 'clear explicit' : 'preserve omitted'} content',
      () {
        final client = ClientUpdateMapper(session, turn);
        final replay = ClientTimelineMapper(session);
        final conversation = ClientConversationMapper(session);
        final progress = acp.ToolCallStatusUpdate(
          update: acp.ToolCallUpdate(
            toolCallId: 'call',
            status: acp.ToolCallStatus.inProgress,
            content: [
              acp.ToolCallContentBlock(
                content: acp.TextContentBlock(text: 'build complete'),
              ),
            ],
          ),
        );
        client.map(progress);
        replay.map(progress);
        conversation.map(progress);
        final metadataOnly = acp.ToolCallStatusUpdate(
          update: acp.ToolCallUpdate(
            toolCallId: 'call',
            rawOutput: {
              'metadata': {'exit_code': 0},
            },
          ),
        );
        final intermediate = client
            .map(metadataOnly)
            .whereType<rt.ToolOutputUpdated>();
        expect(
          intermediate.every(
            (event) => event.output.content == 'build complete',
          ),
          isTrue,
        );
        replay.map(metadataOnly);
        conversation.map(metadataOnly);
        final finish = acp.ToolCallStatusUpdate(
          update: acp.ToolCallUpdate(
            toolCallId: 'call',
            status: acp.ToolCallStatus.completed,
            content: clear ? [] : null,
          ),
        );
        final result = client
            .map(finish)
            .whereType<rt.ToolFinished>()
            .single
            .result;
        final restored = replay
            .map(finish)
            .whereType<rt.ToolResultItem>()
            .single;
        expect(result.content, clear ? '' : 'build complete');
        expect(restored.content, result.content);
        expect(
          conversation
              .map(finish)
              .whereType<rt.ConversationToolResult>()
              .single
              .content,
          result.content,
        );
        expect(result.metadata['exit_code'], 0);
        expect(restored.metadata, result.metadata);
      },
    );
  }

  test('replays shell errors as portable text with metadata', () {
    final result = rt.ToolResultItem(
      id: rt.TimelineItemId('result'),
      sessionId: session,
      turnId: turn,
      sequence: 1,
      occurredAt: now,
      callId: call.call.id,
      content: 'command cancelled',
      isError: true,
      metadata: const {'termination_reason': 'cancelled'},
    );
    final mapper = ClientTimelineMapper(session);
    final items = replayTimeline([call, result]).expand(mapper.map).toList();
    final replayed = items.whereType<rt.ToolResultItem>().single;
    expect(replayed.isError, isTrue);
    expect(replayed.content, 'command cancelled');
    expect(replayed.metadata, result.metadata);
  });

  test('reads standard text content when another ACP peer omits rawOutput', () {
    final mapper = ClientUpdateMapper(session, turn);
    final update = acp.ToolCallStatusUpdate(
      update: acp.ToolCallUpdate(
        toolCallId: 'call',
        status: acp.ToolCallStatus.inProgress,
        kind: acp.ToolKind.execute,
        content: [
          acp.ToolCallContentBlock(content: acp.TextContentBlock(text: 'live')),
        ],
      ),
    );
    final events = mapper.map(update);
    expect(events.map((event) => event.runtimeType), [
      rt.ToolStarted,
      rt.ToolOutputUpdated,
    ]);
    expect(events.last is rt.ToolOutputUpdated, isTrue);
    expect((events.last as rt.ToolOutputUpdated).output.content, 'live');
  });
}

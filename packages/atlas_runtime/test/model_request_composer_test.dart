import 'package:atlas_runtime/src/agent/model_request_composer.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  final sessionId = SessionId('session-1');
  final turn = TurnId('turn-1');

  TimelineItem userItem(int sequence, [String text = 'hello']) =>
      UserMessageItem(
        id: TimelineItemId('u$sequence'),
        sessionId: sessionId,
        turnId: turn,
        sequence: sequence,
        occurredAt: DateTime.utc(2026),
        content: [TextContent(text)],
      );

  AssistantMessageItem assistantItem(
    int sequence, {
    List<ToolCall> toolCalls = const [],
    String text = 'reply',
  }) => AssistantMessageItem(
    id: TimelineItemId('a$sequence'),
    sessionId: sessionId,
    turnId: turn,
    sequence: sequence,
    occurredAt: DateTime.utc(2026),
    content: [TextContent(text)],
    model: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
    stopReason: toolCalls.isEmpty ? StopReason.endTurn : StopReason.toolUse,
  );

  ToolCallItem callItem(int sequence, ToolCall call) => ToolCallItem(
    id: TimelineItemId('c$sequence'),
    sessionId: sessionId,
    turnId: turn,
    sequence: sequence,
    occurredAt: DateTime.utc(2026),
    call: call,
  );

  ToolResultItem resultItem(int sequence, ToolCallId callId) => ToolResultItem(
    id: TimelineItemId('r$sequence'),
    sessionId: sessionId,
    turnId: turn,
    sequence: sequence,
    occurredAt: DateTime.utc(2026),
    callId: callId,
    content: 'result',
  );

  group('projectTimeline', () {
    test('merges tool calls into the assistant message and keeps results', () {
      final call = ToolCall(
        id: ToolCallId('call-1'),
        name: 'read',
        arguments: const <String, Object?>{},
      );
      final messages = ModelRequestComposer.projectTimeline([
        userItem(1),
        assistantItem(2),
        callItem(3, call),
        resultItem(4, ToolCallId('call-1')),
      ], const []);

      expect(messages, hasLength(3));
      expect(messages[0].role, ModelMessageRole.user);
      expect(messages[1].role, ModelMessageRole.assistant);
      expect(messages[1].toolCalls.single.id, ToolCallId('call-1'));
      expect(messages[2].role, ModelMessageRole.tool);
      expect(messages[2].toolCallId, ToolCallId('call-1'));
    });

    test('drops orphan tool calls but keeps the assistant text', () {
      final call = ToolCall(
        id: ToolCallId('call-1'),
        name: 'read',
        arguments: const <String, Object?>{},
      );
      final messages = ModelRequestComposer.projectTimeline([
        assistantItem(1),
        callItem(2, call),
      ], const []);

      expect(messages, hasLength(1));
      expect(messages.single.toolCalls, isEmpty);
      expect(
        messages.single.content.whereType<TextContent>().single.text,
        'reply',
      );
    });

    test('excludes items at or before the compaction boundary', () {
      final compaction = CompactionCheckpoint(
        sessionId: sessionId,
        compactedThroughSequence: 2,
        summary: 'summary',
        keptRecentMessages: 1,
        inputTokensBefore: 10,
        inputTokensAfter: 5,
        createdAt: DateTime.utc(2026),
      );
      final messages = ModelRequestComposer.projectTimeline(
        [userItem(1), userItem(2), userItem(3, 'kept')],
        const [],
        compaction: compaction,
      );

      expect(messages, hasLength(1));
      expect(
        messages.single.content.whereType<TextContent>().single.text,
        'kept',
      );
    });

    test('attaches provider continuations from model checkpoints', () {
      final assistant = assistantItem(1);
      final continuation = ModelContinuation(
        providerId: ProviderId('test'),
        reasoningSummary: 'why',
        opaquePayload: const <String, Object?>{'cursor': 'one'},
      );
      final checkpoint = ModelCheckpoint(
        timelineItemId: assistant.id,
        continuation: continuation,
        createdAt: DateTime.utc(2026),
      );
      final messages = ModelRequestComposer.projectTimeline(
        [assistant],
        [checkpoint],
      );

      expect(messages.single.continuation?.opaquePayload['cursor'], 'one');
    });
  });

  group('applyInputCapabilities', () {
    final withImage = ModelMessage(
      role: ModelMessageRole.user,
      content: [
        const TextContent('look'),
        ImageContent(
          source: 'data:image/png;base64,AAAA',
          mimeType: 'image/png',
        ),
      ],
    );

    test('returns the original list when no filtering is needed', () {
      const messages = <ModelMessage>[];
      expect(
        ModelRequestComposer.applyInputCapabilities(messages, null),
        same(messages),
      );
      final descriptor = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId('t'), modelId: ModelId('m')),
        contextWindow: 1000,
        inputCapabilities: const {
          ModelInputCapability.text,
          ModelInputCapability.image,
        },
      );
      final imageMessages = [withImage];
      expect(
        ModelRequestComposer.applyInputCapabilities(imageMessages, descriptor),
        same(imageMessages),
      );
    });

    test('replaces image parts with the placeholder for text-only models', () {
      final descriptor = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId('t'), modelId: ModelId('m')),
        contextWindow: 1000,
      );
      final filtered = ModelRequestComposer.applyInputCapabilities([
        withImage,
      ], descriptor);

      expect(filtered, hasLength(1));
      expect(filtered.single.content, hasLength(2));
      expect(filtered.single.content[0], isA<TextContent>());
      expect(filtered.single.content[1], isA<TextContent>());
      expect(
        (filtered.single.content[1] as TextContent).text,
        contains('[image omitted'),
      );
    });
  });

  group('skillMessages', () {
    final skill = Skill(
      name: 'reader',
      description: 'Reads things.',
      dir: '/skills/reader',
      path: '/skills/reader/SKILL.md',
      content: 'Body <with> & markup.',
    );
    final catalog = _SingleSkillCatalog(skill);

    test('renders selected skills once, in first-selection order', () {
      final messages = ModelRequestComposer.skillMessages([
        'reader',
        'reader',
        'missing',
      ], catalog);

      expect(messages, hasLength(1));
      final text = messages.single.content.whereType<TextContent>().single.text;
      expect(text, startsWith('<skill>'));
      // The body is injected verbatim; only the metadata is escaped.
      expect(text, contains('Body <with> & markup.'));
    });

    test('escapes XML-significant characters in skill metadata', () {
      final tricky = Skill(
        name: 'a<b>&c',
        description: 'Tricky.',
        dir: '/skills/tricky',
        path: '/skills/tricky/SKILL.md',
        content: 'Body',
      );
      final messages = ModelRequestComposer.skillMessages([
        'a<b>&c',
      ], _SingleSkillCatalog(tricky));

      final text = messages.single.content.whereType<TextContent>().single.text;
      expect(text, contains('<name>a&lt;b&gt;&amp;c</name>'));
    });

    test('rejects selections exceeding the byte budget', () {
      final oversized = Skill(
        name: 'big',
        description: 'Big.',
        dir: '/skills/big',
        path: '/skills/big/SKILL.md',
        content: 'x' * (maxSelectedSkillBytes + 1),
      );
      expect(
        () => ModelRequestComposer.skillMessages([
          'big',
        ], _SingleSkillCatalog(oversized)),
        throwsStateError,
      );
    });
  });
}

/// A catalog holding exactly one lookup-able skill.
final class _SingleSkillCatalog implements SkillCatalog {
  _SingleSkillCatalog(this.skill);

  final Skill skill;

  @override
  List<SkillSummary> get summaries => [
    SkillSummary(
      name: skill.name,
      path: skill.path,
      description: skill.description,
    ),
  ];

  @override
  Skill? lookup(String name) => name == skill.name ? skill : null;
}

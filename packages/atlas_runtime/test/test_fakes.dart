import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';

final testModel = ModelRef(
  providerId: ProviderId('test'),
  modelId: ModelId('model'),
);

final class TestIds implements IdGenerator {
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

final class ScriptedProvider implements ModelProvider {
  ScriptedProvider(
    this.responses, {
    this.contextWindow = 0,
    this.inputCapabilities = const <ModelInputCapability>{
      ModelInputCapability.text,
    },
  });

  final List<ModelResponse> responses;
  final int contextWindow;
  final Set<ModelInputCapability> inputCapabilities;
  final requests = <ModelRequest>[];
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async => ModelDescriptor(
    ref: model,
    contextWindow: contextWindow,
    inputCapabilities: inputCapabilities,
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    yield const TextDeltaEvent('delta');
    yield ModelCompletedEvent(responses[_index++]);
  }
}

final class FailingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield ModelFailedEvent(StateError('provider failed'), StackTrace.current);
  }
}

/// Succeeds for the scripted responses, then fails further model calls.
final class SummaryFailingProvider implements ModelProvider {
  SummaryFailingProvider(this.responses);

  final List<ModelResponse> responses;
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: 10000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (_index < responses.length) {
      yield ModelCompletedEvent(responses[_index++]);
      return;
    }
    yield ModelFailedEvent(StateError('summary failed'), StackTrace.current);
  }
}

/// Fails every describe call while streaming scripted responses.
final class DescribeFailingProvider implements ModelProvider {
  DescribeFailingProvider(this.responses);

  final List<ModelResponse> responses;
  final requests = <ModelRequest>[];
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      throw StateError('describe failed');

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    yield const TextDeltaEvent('delta');
    yield ModelCompletedEvent(responses[_index++]);
  }
}

/// Succeeds once, then fails every subsequent model call.
final class FirstOkThenFailProvider implements ModelProvider {
  FirstOkThenFailProvider(this.first);

  final ModelResponse first;
  var _calls = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: 100);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (_calls++ == 0) {
      yield ModelCompletedEvent(first);
      return;
    }
    yield ModelFailedEvent(StateError('provider failed'), StackTrace.current);
  }
}

final class BlockingProvider implements ModelProvider {
  final firstRequestStarted = Completer<void>();
  final releaseFirst = Completer<void>();
  final requests = <ModelRequest>[];

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    if (requests.length == 1) {
      firstRequestStarted.complete();
      await releaseFirst.future;
    }
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('done')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class CancellingProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    request.cancellation!.cancel();
    yield const TextDeltaEvent('ignored');
  }
}

/// Emits one text delta, then cancels the request mid-stream.
final class PartialStreamCancelProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield const TextDeltaEvent('partial answer');
    request.cancellation!.cancel();
    throw const TurnCancelledException();
  }
}

/// Completes one tool-use step, then cancels the next stream mid-answer.
final class CancelAfterToolUseProvider implements ModelProvider {
  var _step = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (_step++ == 0) {
      yield ModelCompletedEvent(
        ModelResponse(
          content: const [TextContent('calling tool')],
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'inspect',
              arguments: const <String, Object?>{},
            ),
          ],
          stopReason: StopReason.toolUse,
          usage: const TokenUsage(inputTokens: 10, outputTokens: 5),
        ),
      );
      return;
    }
    yield const TextDeltaEvent('partial answer');
    request.cancellation!.cancel();
    throw const TurnCancelledException();
  }
}

/// A session context builder that injects [skills] for every directory.
SessionContext Function(String) contextBuilder(SkillCatalog skills) =>
    (cwd) => SessionContext(
      workingDirectory: cwd,
      instructions: const [],
      skills: skills,
    );

final class MemorySkillCatalog implements SkillCatalog {
  MemorySkillCatalog(this.skills);

  final List<Skill> skills;

  @override
  List<SkillSummary> get summaries => [
    for (final skill in skills)
      if (!skill.disableModelInvocation)
        SkillSummary(
          name: skill.name,
          path: skill.path,
          description: skill.description,
        ),
  ];

  @override
  Skill? lookup(String name) {
    for (final skill in skills) {
      if (skill.name == name) {
        return skill.disableModelInvocation ? null : skill;
      }
    }
    return null;
  }
}

final class MemoryTools implements ToolRegistry {
  MemoryTools({required this.result});

  final ToolResult result;
  final contexts = <ToolContext>[];
  final calls = <ToolCall>[];

  @override
  List<ToolDescriptor> get descriptors => const [
    ToolDescriptor(
      name: 'inspect',
      description: 'Inspect files',
      inputSchema: <String, Object?>{},
    ),
  ];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async {
    contexts.add(context);
    calls.add(call);
    return result;
  }
}

/// Cancels after its first call to exercise result pairing for queued calls.
final class CancellingTools implements ToolRegistry {
  final calls = <ToolCall>[];

  @override
  List<ToolDescriptor> get descriptors => const [
    ToolDescriptor(
      name: 'inspect',
      description: 'Inspect files',
      inputSchema: <String, Object?>{},
    ),
  ];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async {
    calls.add(call);
    context.cancellation!.cancel();
    return const ToolResult(content: 'first result');
  }
}

final class ThrowingTools implements ToolRegistry {
  @override
  List<ToolDescriptor> get descriptors => const [];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async {
    throw StateError('tool failed');
  }
}

final class MemorySessionStore implements SessionStore {
  Session? session;
  final turns = <Turn>[];
  final timeline = <TimelineItem>[];
  final checkpoints = <ModelCheckpoint>[];
  CompactionCheckpoint? compaction;

  @override
  Future<void> createSession(Session value) async => session = value;

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) async {
    final value = session;
    if (value == null || value.id != sessionId) {
      throw SessionNotFoundException(sessionId);
    }
    final checkpoint = value.compaction;
    final visible = checkpoint == null || checkpoint.summary.trim().isEmpty
        ? timeline
        : timeline
              .where(
                (item) => item.sequence > checkpoint.compactedThroughSequence,
              )
              .toList();
    return SessionSnapshot(
      session: value,
      turns: List.unmodifiable(turns),
      timeline: List.unmodifiable(visible),
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
    if (timeline.any(
      (item) => item.sequence == operation.userMessage.sequence,
    )) {
      throw StateError('duplicate timeline sequence');
    }
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
    compaction = checkpoint;
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

/// Rejects every model-step append to exercise cancellation-path tolerance.
final class RejectingStepStore extends MemorySessionStore {
  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) async {
    throw SessionNotFoundException(sessionId);
  }
}

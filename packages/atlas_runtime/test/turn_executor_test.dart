import 'package:atlas_runtime/src/agent/context_compactor.dart';
import 'package:atlas_runtime/src/agent/turn_executor.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  final model = ModelRef(providerId: ProviderId('test'), modelId: ModelId('m'));

  TurnExecutor executor(
    _MemorySessionStore store,
    _ScriptedProvider provider, {
    ContextCompactor? compactor,
  }) => TurnExecutor(
    store: store,
    provider: provider,
    tools: _MemoryTools(
      result: const ToolResult(content: 'file list', isError: false),
    ),
    ids: _Ids(),
    logger: const NoopLogger(),
    defaultModel: model,
    compactor:
        compactor ??
        ContextCompactor(
          provider: provider,
          store: store,
          threshold: 0.8,
          keptRecentTurns: 5,
        ),
    sessionContextOf: (workingDirectory) => SessionContext(
      workingDirectory: workingDirectory,
      instructions: const [],
      skills: _EmptyCatalog(),
    ),
    systemPromptBuilder: (context) => 'system prompt',
  );

  test('runs the tool loop in occurrence order without the facade', () async {
    final store = _MemorySessionStore();
    final provider = _ScriptedProvider([
      ModelResponse(
        content: const [TextContent('Inspecting.')],
        toolCalls: [
          ToolCall(
            id: ToolCallId('call-1'),
            name: 'inspect',
            arguments: const <String, Object?>{},
          ),
        ],
        stopReason: StopReason.toolUse,
      ),
      const ModelResponse(
        content: [TextContent('Done.')],
        stopReason: StopReason.endTurn,
      ),
    ], contextWindow: 10000);

    final events = await executor(store, provider)
        .run(
          const TurnRequest(
            content: [TextContent('Inspect the files')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    expect(events.map((event) => event.runtimeType), [
      TurnStarted,
      ModelTextDelta,
      ModelResponseReceived,
      ToolStarted,
      ToolFinished,
      ModelTextDelta,
      ModelResponseReceived,
      TurnFinished,
    ]);
    // Every tool call is paired with exactly one persisted result.
    expect(
      store.timeline.whereType<ToolCallItem>().map((item) => item.call.id),
      store.timeline
          .whereType<ToolResultItem>()
          .map((item) => item.callId)
          .toList(),
    );
    expect(store.turns.single.status, TurnStatus.completed);
  });

  test(
    'manual compaction uses the latest turn without the threshold',
    () async {
      final store = _MemorySessionStore();
      final provider = _ScriptedProvider([
        const ModelResponse(
          content: [TextContent('First reply')],
          stopReason: StopReason.endTurn,
        ),
        const ModelResponse(
          content: [TextContent('Second reply')],
          stopReason: StopReason.endTurn,
        ),
        const ModelResponse(
          content: [TextContent('Summary.')],
          stopReason: StopReason.endTurn,
        ),
      ], contextWindow: 10000);
      final turnExecutor = executor(store, provider);
      await turnExecutor
          .run(
            const TurnRequest(
              content: [TextContent('First turn')],
              workingDirectory: '/tmp',
            ),
          )
          .toList();

      await turnExecutor
          .run(
            TurnRequest(
              sessionId: store.session!.id,
              content: const [TextContent('Second turn')],
            ),
          )
          .toList();

      final snapshot = await store.loadSession(store.session!.id);
      final events = await turnExecutor
          .compact(snapshot, instruction: 'Focus on the database.')
          .toList();

      expect(events.map((event) => event.runtimeType), [
        CompactionStarted,
        CompactionFinished,
      ]);
      final prompt = provider.requests.last.messages.single.content
          .whereType<TextContent>()
          .single
          .text;
      expect(
        prompt,
        contains('Additional user instruction:\nFocus on the database.'),
      );
    },
  );
}

final class _ScriptedProvider implements ModelProvider {
  _ScriptedProvider(this.responses, {this.contextWindow = 0});

  final List<ModelResponse> responses;
  final int contextWindow;
  final requests = <ModelRequest>[];
  var _index = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef described) async => ModelDescriptor(
    ref: described,
    contextWindow: contextWindow,
    inputCapabilities: const {ModelInputCapability.text},
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests.add(request);
    yield const TextDeltaEvent('delta');
    yield ModelCompletedEvent(responses[_index++]);
  }
}

final class _MemoryTools implements ToolRegistry {
  _MemoryTools({required this.result});

  final ToolResult result;

  @override
  List<ToolDescriptor> get descriptors => const [
    ToolDescriptor(
      name: 'inspect',
      description: 'Inspect files',
      inputSchema: <String, Object?>{},
    ),
  ];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async =>
      result;
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

final class _MemorySessionStore implements SessionStore {
  Session? session;
  final turns = <Turn>[];
  final timeline = <TimelineItem>[];

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
      modelCheckpoints: const [],
    );
  }

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
  }

  @override
  Future<void> appendToolResult(
    SessionId sessionId,
    ToolResultItem item,
  ) async => timeline.add(item);

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) async =>
      turns[turns.indexWhere((item) => item.id == turn.id)] = turn;

  @override
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  ) async {}

  @override
  Future<SessionPage> listSessions(SessionQuery query) async =>
      const SessionPage(items: []);

  @override
  Future<void> deleteSession(SessionId sessionId) async {}

  @override
  Future<void> renameSession(SessionId sessionId, String title) async {}
}

/// A catalog with no skills, sufficient for context resolution in tests.
final class _EmptyCatalog implements SkillCatalog {
  @override
  List<SkillSummary> get summaries => const [];

  @override
  Skill? lookup(String name) => null;
}

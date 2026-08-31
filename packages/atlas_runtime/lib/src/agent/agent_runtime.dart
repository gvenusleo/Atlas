import 'dart:async';

import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/instruction_file.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/session_context.dart';
import '../domain/turn.dart';
import '../ports/cancellation.dart';
import '../ports/id_generator.dart';
import '../ports/model_provider.dart';
import '../ports/session_store.dart';
import '../ports/tool_registry.dart';
import '../ports/logger.dart';
import '../skills/skill.dart';
import '../skills/skill_catalog.dart';
import 'agent_engine.dart';
import 'agent_session.dart';
import 'agent_capabilities.dart';
import 'context_compactor.dart';
import 'turn_executor.dart';

/// Executes model turns and persists every durable boundary through ports.
final class AgentRuntime
    implements AgentEngine, PresentationAgentSession, AgentCapabilityProvider {
  /// Creates an agent runtime with injected model, tool, and storage adapters.
  AgentRuntime({
    required this.store,
    required this.provider,
    required this.tools,
    required this.ids,
    required this.defaultModel,
    this.sessionContextBuilder = _emptySessionContext,
    this.logger = const NoopLogger(),
    DateTime Function()? now,
    this.systemPromptBuilder = _emptySystemPrompt,
    this.maxSteps = 20,
    this.maxOutputTokens = 0,
    this.temperature,
    this.compactionThreshold = 0.8,
    this.keepRecentTokens,
    this.reserveTokens = 16384,
    this.keptRecentTurns = 5,
  }) : _now = now ?? DateTime.now,
       _compactor = ContextCompactor(
         provider: provider,
         store: store,
         threshold: compactionThreshold,
         keepRecentTokens:
             keepRecentTokens ?? (keptRecentTurns == 5 ? 20000 : null),
         reserveTokens: reserveTokens,
         keptRecentTurns: keptRecentTurns,
         now: now,
       );

  /// The session persistence adapter.
  final SessionStore store;

  /// The model provider adapter.
  final ModelProvider provider;

  /// The registered local tools.
  final ToolRegistry tools;

  /// Builds the filesystem context (instructions and skills) for a session
  /// working directory; invoked once per directory and cached.
  final SessionContext Function(String workingDirectory) sessionContextBuilder;

  /// Structured diagnostic logger.
  final AtlasLogger logger;

  @override
  SessionContext sessionContext(String workingDirectory) =>
      sessionContextBuilder(workingDirectory);

  @override
  Future<void> updateSessionConfig(
    SessionId sessionId,
    ModelRef model,
    String? reasoningEffort,
  ) async {
    if (store case final SessionConfigStore configurable) {
      await configurable.updateSessionConfig(sessionId, model, reasoningEffort);
    }
  }

  /// The ID generator used for new records.
  final IdGenerator ids;

  /// The model used when a turn does not provide an override.
  @override
  final ModelRef defaultModel;

  /// Builds the system prompt for a session and turn.
  final String Function(SessionContext context) systemPromptBuilder;

  /// Maximum model/tool steps for one turn.
  final int maxSteps;

  /// Model output token limit.
  final int maxOutputTokens;

  /// Optional model temperature.
  final double? temperature;

  /// Context window fraction that triggers compaction after a turn.
  final double compactionThreshold;

  /// The approximate number of newest tokens kept verbatim.
  final int? keepRecentTokens;

  /// Tokens reserved for the next model response.
  final int reserveTokens;

  /// Legacy turn count retained for source compatibility.
  final int keptRecentTurns;

  final DateTime Function() _now;
  final ContextCompactor _compactor;

  /// The turn loop, sharing this runtime's ports, cache, and compactor.
  late final TurnExecutor _executor = TurnExecutor(
    store: store,
    provider: provider,
    tools: tools,
    ids: ids,
    logger: logger,
    defaultModel: defaultModel,
    compactor: _compactor,
    sessionContextOf: _contextFor,
    systemPromptBuilder: systemPromptBuilder,
    now: _now,
    maxSteps: maxSteps,
    maxOutputTokens: maxOutputTokens,
    temperature: temperature,
  );
  final Map<SessionId, Future<void>> _sessionTails = {};
  final Map<String, SessionContext> _sessionContexts = {};
  final Map<SessionId, List<AgentCommand>> _sessionCommands = {};

  @override
  AgentCapabilities get capabilities =>
      const AgentCapabilities(slashCommands: true, compact: true);

  /// Executes one turn and emits events in their exact occurrence order.
  @override
  Stream<AgentEvent> run(TurnRequest request) async* {
    final sessionId = request.sessionId;
    if (sessionId == null) {
      yield* _executor.run(request);
      return;
    }
    final release = await _acquireSessionLock(sessionId);
    try {
      yield* _executor.run(request);
    } finally {
      release();
    }
  }

  /// Manually compacts [sessionId] without the threshold check.
  ///
  /// [instruction] is an optional user-provided direction for the compaction
  /// summary; when non-empty it is included in the summary request.
  /// [cancellation] stops the summary model request cooperatively.
  ///
  /// Uses the model and turn of the latest recorded turn so the emitted
  /// events stay attached to the session's most recent execution.
  @override
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    CancellationToken? cancellation,
  }) async* {
    final release = await _acquireSessionLock(sessionId);
    try {
      cancellation?.throwIfCancelled();
      final snapshot = await store.loadSession(sessionId);
      yield* _executor.compact(
        snapshot,
        instruction: instruction,
        cancellation: cancellation,
      );
    } finally {
      release();
    }
  }

  /// Lists session summaries in descending update order.
  ///
  /// Passes [workingDirectory] to restrict results to one directory, or
  /// `null` to list every session. See [SessionQuery] for pagination.
  @override
  Future<SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  }) => store.listSessions(
    SessionQuery(
      workingDirectory: workingDirectory,
      cursor: cursor,
      limit: limit,
    ),
  );

  /// Creates a new blank session and persists it.
  ///
  /// Used by protocol adapters whose session lifecycle is separate from the
  /// first turn, such as ACP's `session/new`.
  @override
  Future<Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const <String>[],
  }) async {
    if (workingDirectory.trim().isEmpty) {
      throw ArgumentError('workingDirectory is required for a new session');
    }
    final now = _now().toUtc();
    final session = Session(
      id: ids.sessionId(),
      workingDirectory: workingDirectory,
      additionalDirectories: List<String>.unmodifiable(additionalDirectories),
      createdAt: now,
      updatedAt: now,
    );
    await store.createSession(session);
    _sessionCommands[session.id] = [
      for (final skill in sessionContext(workingDirectory).skills.summaries)
        AgentCommand(name: skill.name, description: skill.description),
    ];
    return session;
  }

  /// Loads one session and its timeline for display or resume.
  ///
  /// Throws [SessionNotFoundException] when [sessionId] does not exist.
  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      store.loadSession(sessionId);

  /// Loads lightweight session metadata for presentation updates.
  @override
  Future<Session> loadSessionMetadata(SessionId sessionId) =>
      store is SessionMetadataStore
      ? (store as SessionMetadataStore).loadSessionMetadata(sessionId)
      : store.loadSession(sessionId).then((snapshot) => snapshot.session);

  /// Deletes [sessionId] and all of its dependent records.
  ///
  /// Throws [SessionNotFoundException] when [sessionId] does not exist.
  @override
  Future<void> deleteSession(SessionId sessionId) async {
    final release = await _acquireSessionLock(sessionId);
    try {
      await store.deleteSession(sessionId);
    } finally {
      release();
    }
  }

  /// Renames [sessionId]'s display title.
  ///
  /// Throws [SessionNotFoundException] when [sessionId] does not exist.
  @override
  Future<void> renameSession(SessionId sessionId, String title) async {
    final release = await _acquireSessionLock(sessionId);
    try {
      await store.renameSession(sessionId, title);
    } finally {
      release();
    }
  }

  /// The context window size of the default model, or 0 when unknown.
  @override
  Future<int> contextWindowSize() async {
    try {
      return (await provider.describe(defaultModel)).contextWindow;
    } catch (_) {
      return 0;
    }
  }

  /// Local sessions already carry their persisted title.
  @override
  String? titleFor(SessionId sessionId) => null;

  /// Local slash commands come from the skill catalog, not the runtime.
  @override
  List<AgentCommand> commandsFor(SessionId sessionId) =>
      _sessionCommands[sessionId] ?? const [];

  /// Local sessions have no agent-defined operating modes.
  @override
  List<ModeOption> get modeOptions => const [];

  /// Local sessions have no agent-defined operating modes.
  @override
  String? modeFor(SessionId sessionId) => null;

  /// Local sessions have no agent-defined operating modes.
  @override
  Future<void> setMode(SessionId sessionId, String modeId) async {}

  /// The cached session context for a working directory.
  SessionContext _contextFor(String workingDirectory) =>
      _sessionContexts.putIfAbsent(
        workingDirectory,
        () => sessionContextBuilder(workingDirectory),
      );

  static String _emptySystemPrompt(SessionContext context) => '';

  /// A default context builder for runtimes that do not load filesystem
  /// context; returns an empty context per working directory.
  static SessionContext _emptySessionContext(String workingDirectory) =>
      SessionContext(
        workingDirectory: workingDirectory,
        instructions: const <InstructionFile>[],
        skills: _EmptySkillCatalog(),
      );

  Future<void Function()> _acquireSessionLock(SessionId sessionId) async {
    final previous = _sessionTails[sessionId];
    final releaseCompleter = Completer<void>();
    final current = releaseCompleter.future;
    _sessionTails[sessionId] = current;
    if (previous != null) {
      await previous;
    }
    return () {
      if (releaseCompleter.isCompleted) {
        return;
      }
      releaseCompleter.complete();
      if (identical(_sessionTails[sessionId], current)) {
        _sessionTails.remove(sessionId);
      }
    };
  }
}

/// A skill catalog with no skills, used when no context builder is provided.
final class _EmptySkillCatalog implements SkillCatalog {
  @override
  List<SkillSummary> get summaries => const [];

  @override
  Skill? lookup(String name) => null;
}

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
final class AgentRuntime({
  /// The session persistence adapter.
  required final SessionStore store,

  /// The model provider adapter.
  required final ModelProvider provider,

  /// The registered local tools.
  required final ToolRegistry tools,

  /// The ID generator used for new records.
  required final IdGenerator ids,

  /// The model used when a turn does not provide an override.
  @override required final ModelRef defaultModel,

  /// Builds the filesystem context (instructions and skills) for a session
  /// working directory; invoked once per directory and cached.
  final SessionContext Function(String workingDirectory) sessionContextBuilder =
      _emptySessionContext,

  /// Structured diagnostic logger.
  final AtlasLogger logger = const NoopLogger(),
  DateTime Function()? now,

  /// Builds the system prompt for a session and turn.
  final String Function(SessionContext context) systemPromptBuilder =
      _emptySystemPrompt,

  /// Maximum model/tool steps for one turn.
  final int maxSteps = 20,

  /// Model output token limit.
  final int maxOutputTokens = 0,

  /// Optional model temperature.
  final double? temperature,

  /// Context window fraction that triggers compaction after a turn.
  final double compactionThreshold = 0.8,

  /// The approximate number of newest tokens kept verbatim.
  final int? keepRecentTokens,

  /// Tokens reserved for the next model response.
  final int reserveTokens = 16384,

  /// Legacy turn count retained for source compatibility.
  final int keptRecentTurns = 5,
}) implements AgentEngine, PresentationAgentSession, AgentCapabilityProvider {
  /// Creates an agent runtime with injected model, tool, and storage adapters.
  this
    : _now = now ?? DateTime.now,
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
  final Map<Completer<void>, CancellationToken> _operations = {};
  bool _shuttingDown = false;

  /// Cancels active and queued turns/compactions and waits for persistence.
  ///
  /// Callers must stop accepting requests first and keep consuming event
  /// streams until they finish. Adapter resources remain owned by the caller.
  Future<void> shutdown() async {
    _shuttingDown = true;
    final pending = _operations.entries.toList();
    for (final entry in pending) {
      entry.value.cancel();
    }
    await Future.wait(pending.map((entry) => entry.key.future));
  }

  Completer<void> _beginOperation(CancellationToken cancellation) {
    if (_shuttingDown) throw StateError('Runtime is shutting down');
    final done = Completer<void>();
    _operations[done] = cancellation;
    return done;
  }

  void _endOperation(Completer<void> done) {
    _operations.remove(done);
    done.complete();
  }

  final Map<SessionId, Future<void>> _sessionTails = {};
  final Map<String, SessionContext> _sessionContexts = {};
  final Map<SessionId, List<AgentCommand>> _sessionCommands = {};

  @override
  AgentCapabilities get capabilities =>
      const AgentCapabilities(slashCommands: true, compact: true);

  /// Executes one turn and emits events in their exact occurrence order.
  @override
  Stream<AgentEvent> run(TurnRequest request) async* {
    final cancellation = request.cancellation ?? CancellationToken();
    final done = _beginOperation(cancellation);
    void Function()? release;
    try {
      final sessionId = request.sessionId;
      if (sessionId != null) release = await _acquireSessionLock(sessionId);
      cancellation.throwIfCancelled();
      yield* _executor.run(request, cancellation: cancellation);
    } finally {
      release?.call();
      _endOperation(done);
    }
  }

  /// Manually compacts [sessionId] without the threshold check.
  ///
  /// [instruction] is an optional user-provided direction for the compaction
  /// summary; when non-empty it is included in the summary request.
  /// [model] overrides the session's selected model for the summary request.
  /// [cancellation] stops the summary model request cooperatively.
  ///
  /// The summary runs on the session's selected model, falling back to the
  /// model of the latest recorded turn and then to the runtime default.
  @override
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    ModelRef? model,
    CancellationToken? cancellation,
  }) async* {
    final token = cancellation ?? CancellationToken();
    final done = _beginOperation(token);
    void Function()? release;
    try {
      release = await _acquireSessionLock(sessionId);
      token.throwIfCancelled();
      final snapshot = await store.loadSession(sessionId);
      yield* _executor.compact(
        snapshot,
        instruction: instruction,
        model: model,
        cancellation: token,
      );
    } finally {
      release?.call();
      _endOperation(done);
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

  /// The context window size of [model], or of the default model when
  /// [model] is omitted, or 0 when unknown.
  @override
  Future<int> contextWindowSize({ModelRef? model}) async {
    try {
      return (await provider.describe(model ?? defaultModel)).contextWindow;
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

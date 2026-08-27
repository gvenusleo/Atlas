import 'dart:async';
import 'dart:convert';

import '../domain/content.dart';
import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/instruction_file.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/session_context.dart';
import '../domain/timeline.dart';
import '../domain/turn.dart';
import '../domain/usage.dart';
import '../ports/cancellation.dart';
import '../ports/failures.dart';
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

/// Executes model turns and persists every durable boundary through ports.
final class AgentRuntime
    implements AgentEngine, AgentSession, AgentCapabilityProvider {
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
    this.keptRecentTurns = 5,
  }) : _now = now ?? DateTime.now;

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

  /// The number of most recent turns kept verbatim during compaction.
  final int keptRecentTurns;

  final DateTime Function() _now;
  final Map<SessionId, Future<void>> _sessionTails = {};
  final Map<String, SessionContext> _sessionContexts = {};

  @override
  AgentCapabilities get capabilities =>
      const AgentCapabilities(slashCommands: true, compact: true);

  /// Executes one turn and emits events in their exact occurrence order.
  @override
  Stream<AgentEvent> run(TurnRequest request) async* {
    final sessionId = request.sessionId;
    if (sessionId == null) {
      yield* _runTurn(request);
      return;
    }
    final release = await _acquireSessionLock(sessionId);
    try {
      yield* _runTurn(request);
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
      final turns = snapshot.turns;
      if (turns.isEmpty) {
        return;
      }
      final lastTurn = turns.last;
      var eventSequence = 0;
      yield* _compactContext(
        session: snapshot.session,
        timeline: snapshot.timeline,
        context: _contextOrNull(snapshot.session.workingDirectory),
        model: lastTurn.model ?? defaultModel,
        turnId: lastTurn.id,
        latestUsage: lastTurn.usage,
        enforceThreshold: false,
        instruction: instruction,
        cancellation: cancellation,
        nextSequence: () => eventSequence++,
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
    return session;
  }

  /// Loads one session and its timeline for display or resume.
  ///
  /// Throws [SessionNotFoundException] when [sessionId] does not exist.
  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      store.loadSession(sessionId);

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
  List<AgentCommand> commandsFor(SessionId sessionId) => const [];

  /// Local sessions have no agent-defined operating modes.
  @override
  List<ModeOption> get modeOptions => const [];

  /// Local sessions have no agent-defined operating modes.
  @override
  String? modeFor(SessionId sessionId) => null;

  /// Local sessions have no agent-defined operating modes.
  @override
  Future<void> setMode(SessionId sessionId, String modeId) async {}

  Stream<AgentEvent> _runTurn(TurnRequest request) async* {
    final cancellation = request.cancellation ?? CancellationToken();
    final now = _now().toUtc();
    final model = request.model ?? defaultModel;
    final loaded = await _loadOrCreateSession(request, now);
    final session = loaded.session;
    final turnId = ids.turnId();
    final turn = Turn(
      id: turnId,
      sessionId: session.id,
      status: TurnStatus.running,
      startedAt: now,
      model: model,
      reasoningEffort: request.reasoningEffort,
    );
    final sequence = _nextSequence(
      loaded.timeline,
      compaction: session.compaction,
    );
    final userMessage = UserMessageItem(
      id: ids.timelineItemId(),
      sessionId: session.id,
      turnId: turnId,
      sequence: sequence,
      occurredAt: now,
      content: List<ContentPart>.unmodifiable(request.content),
    );

    cancellation.throwIfCancelled();
    await store.beginTurn(
      BeginTurn(session: session, turn: turn, userMessage: userMessage),
    );
    final timeline = <TimelineItem>[...loaded.timeline, userMessage];
    final modelCheckpoints = <ModelCheckpoint>[...loaded.modelCheckpoints];
    var eventSequence = 0;
    yield TurnStarted(
      sessionId: session.id,
      turnId: turnId,
      sequence: eventSequence++,
      occurredAt: _now().toUtc(),
      userMessage: userMessage,
    );

    var latestUsage = const TokenUsage();
    var finalContent = const <ContentPart>[];
    try {
      final context = _contextFor(session.workingDirectory);
      final selectedSkillMessages = _skillMessages(
        request.skills,
        context.skills,
      );
      // Resolved once per turn so historical images can be omitted for
      // models that cannot accept them; null falls back to provider-side
      // request validation.
      final capabilities = await _modelCapabilities(model);
      for (var step = 0; step < maxSteps; step++) {
        cancellation.throwIfCancelled();
        ModelResponse? response;
        final modelRequest = ModelRequest(
          sessionId: session.id,
          turnId: turn.id,
          model: model,
          messages: _applyInputCapabilities([
            ...selectedSkillMessages,
            ..._projectTimeline(
              timeline,
              modelCheckpoints,
              compaction: session.compaction,
            ),
          ], capabilities),
          systemPrompt: _systemPrompt(session, context),
          tools: tools.descriptors,
          reasoningEffort: request.reasoningEffort,
          maxOutputTokens: maxOutputTokens,
          temperature: temperature,
          cancellation: cancellation,
        );
        await for (final event in provider.stream(modelRequest)) {
          cancellation.throwIfCancelled();
          switch (event) {
            case TextDeltaEvent(:final delta):
              yield ModelTextDelta(
                sessionId: session.id,
                turnId: turnId,
                sequence: eventSequence++,
                occurredAt: _now().toUtc(),
                delta: delta,
              );
            case ReasoningDeltaEvent(:final delta):
              yield ModelReasoningDelta(
                sessionId: session.id,
                turnId: turnId,
                sequence: eventSequence++,
                occurredAt: _now().toUtc(),
                delta: delta,
              );
            case ModelCompletedEvent(response: final completed):
              if (response != null) {
                throw StateError('model stream completed more than once');
              }
              response = completed;
            case ModelFailedEvent(:final error, :final stackTrace):
              Error.throwWithStackTrace(error, stackTrace);
          }
        }
        final completedResponse = response;
        if (completedResponse == null) {
          throw StateError('model stream ended without response');
        }

        final assistantId = ids.timelineItemId();
        final assistant = AssistantMessageItem(
          id: assistantId,
          sessionId: session.id,
          turnId: turnId,
          sequence: _nextSequence(timeline),
          occurredAt: _now().toUtc(),
          content: List<ContentPart>.unmodifiable(completedResponse.content),
          model: model,
          stopReason: completedResponse.stopReason,
          reasoning: completedResponse.reasoning,
          usage: completedResponse.usage,
        );
        final calls = <ToolCallItem>[];
        var nextSequence = assistant.sequence + 1;
        for (final call in completedResponse.toolCalls) {
          calls.add(
            ToolCallItem(
              id: ids.timelineItemId(),
              sessionId: session.id,
              turnId: turnId,
              sequence: nextSequence++,
              occurredAt: _now().toUtc(),
              call: call,
            ),
          );
        }
        final checkpoint = completedResponse.continuation == null
            ? null
            : ModelCheckpoint(
                timelineItemId: assistant.id,
                continuation: completedResponse.continuation!,
                createdAt: _now().toUtc(),
              );
        await store.appendModelStep(
          session.id,
          PersistedModelStep(
            assistantMessage: assistant,
            toolCalls: calls,
            checkpoint: checkpoint,
          ),
        );
        timeline.add(assistant);
        timeline.addAll(calls);
        if (checkpoint != null) {
          modelCheckpoints.add(checkpoint);
        }
        latestUsage = completedResponse.usage;
        finalContent = completedResponse.content;
        yield ModelResponseReceived(
          sessionId: session.id,
          turnId: turnId,
          sequence: eventSequence++,
          occurredAt: _now().toUtc(),
          assistantMessage: assistant,
          toolCalls: List<ToolCallItem>.unmodifiable(calls),
        );

        if (calls.isEmpty) {
          final outcome = TurnOutcome(
            sessionId: session.id,
            turnId: turnId,
            status: TurnStatus.completed,
            content: finalContent,
            usage: latestUsage,
            stopReason: completedResponse.stopReason,
          );
          await _finishTurnToleratingDeletion(
            session.id,
            _terminalTurn(turn, TurnStatus.completed, latestUsage),
          );
          yield TurnFinished(
            sessionId: session.id,
            turnId: turnId,
            sequence: eventSequence++,
            occurredAt: _now().toUtc(),
            outcome: outcome,
          );
          yield* _compactContext(
            session: session,
            timeline: timeline,
            context: context,
            model: model,
            turnId: turnId,
            latestUsage: latestUsage,
            nextSequence: () => eventSequence++,
            cancellation: cancellation,
          );
          return;
        }

        for (final callItem in calls) {
          yield ToolStarted(
            sessionId: session.id,
            turnId: turnId,
            sequence: eventSequence++,
            occurredAt: _now().toUtc(),
            call: callItem,
          );
          final result = cancellation.isCancelled
              ? const ToolResult(
                  content: 'Tool execution cancelled',
                  isError: true,
                )
              : await _executeTool(
                  session: session,
                  turn: turn,
                  call: callItem.call,
                  cancellation: cancellation,
                );
          final resultItem = ToolResultItem(
            id: ids.timelineItemId(),
            sessionId: session.id,
            turnId: turnId,
            sequence: _nextSequence(timeline),
            occurredAt: _now().toUtc(),
            callId: callItem.call.id,
            content: result.content,
            isError: result.isError,
            metadata: result.metadata,
          );
          await store.appendToolResult(session.id, resultItem);
          timeline.add(resultItem);
          yield ToolFinished(
            sessionId: session.id,
            turnId: turnId,
            sequence: eventSequence++,
            occurredAt: _now().toUtc(),
            result: resultItem,
          );
        }
      }
      throw StateError('maximum model steps exceeded: $maxSteps');
    } on TurnCancelledException catch (error) {
      final outcome = TurnOutcome(
        sessionId: session.id,
        turnId: turnId,
        status: TurnStatus.cancelled,
        content: finalContent,
        usage: latestUsage,
        failure: TurnFailure(code: 'cancelled', message: error.toString()),
      );
      await _finishTurnToleratingDeletion(
        session.id,
        _terminalTurn(
          turn,
          TurnStatus.cancelled,
          latestUsage,
          cancelReason: error.toString(),
        ),
      );
      yield _finishedEvent(outcome, session.id, turnId, eventSequence++);
      yield* _compactContext(
        session: session,
        timeline: timeline,
        context: _contextOrNull(session.workingDirectory),
        model: model,
        turnId: turnId,
        latestUsage: latestUsage,
        nextSequence: () => eventSequence++,
        cancellation: cancellation,
      );
    } catch (error, stackTrace) {
      final outcome = TurnOutcome(
        sessionId: session.id,
        turnId: turnId,
        status: TurnStatus.failed,
        content: finalContent,
        usage: latestUsage,
        failure: TurnFailure(
          code: 'turn_failed',
          message: _safeErrorMessage('Turn failed', error),
        ),
      );
      logger.log(
        LogEvent(
          level: LogLevel.error,
          code: 'turn.failed',
          message: outcome.failure?.message ?? 'turn failed',
          sessionId: session.id,
          turnId: turnId,
          occurredAt: _now().toUtc(),
        ),
      );
      await _finishTurnToleratingDeletion(
        session.id,
        _terminalTurn(
          turn,
          TurnStatus.failed,
          latestUsage,
          failure: TurnFailure(
            code: 'turn_failed',
            message: _safeErrorMessage('Turn failed', error),
          ),
        ),
      );
      yield _finishedEvent(outcome, session.id, turnId, eventSequence++);
      yield* _compactContext(
        session: session,
        timeline: timeline,
        context: _contextOrNull(session.workingDirectory),
        model: model,
        turnId: turnId,
        latestUsage: latestUsage,
        nextSequence: () => eventSequence++,
        cancellation: cancellation,
      );
      Error.throwWithStackTrace(error, stackTrace);
    }
  }

  Future<
    ({
      Session session,
      List<TimelineItem> timeline,
      List<ModelCheckpoint> modelCheckpoints,
    })
  >
  _loadOrCreateSession(TurnRequest request, DateTime now) async {
    if (request.sessionId != null) {
      final snapshot = await store.loadSession(request.sessionId!);
      final current = snapshot.session;
      final session = Session(
        id: current.id,
        title: current.title,
        workingDirectory: current.workingDirectory,
        additionalDirectories: List<String>.unmodifiable(
          request.additionalDirectories ?? current.additionalDirectories,
        ),
        createdAt: current.createdAt,
        updatedAt: current.updatedAt,
        compaction: current.compaction,
        lastUsage: current.lastUsage,
      );
      return (
        session: session,
        timeline: snapshot.timeline,
        modelCheckpoints: snapshot.modelCheckpoints,
      );
    }
    final workingDirectory = request.workingDirectory;
    if (workingDirectory == null || workingDirectory.trim().isEmpty) {
      throw ArgumentError('workingDirectory is required for a new session');
    }
    final session = Session(
      id: ids.sessionId(),
      workingDirectory: workingDirectory,
      additionalDirectories: List<String>.unmodifiable(
        request.additionalDirectories ?? const <String>[],
      ),
      createdAt: now,
      updatedAt: now,
    );
    return (
      session: session,
      timeline: const <TimelineItem>[],
      modelCheckpoints: const <ModelCheckpoint>[],
    );
  }

  String _systemPrompt(Session session, SessionContext context) {
    final prompt = systemPromptBuilder(context);
    final checkpoint = session.compaction;
    final summary = checkpoint?.summary.trim();
    if (summary == null || summary.isEmpty) {
      return prompt;
    }
    final summaryText =
        'Context compacted. '
        'Kept ${checkpoint!.keptRecentMessages} recent messages.\n\n$summary';
    if (prompt.trim().isEmpty) {
      return '<context_summary>\n$summaryText\n</context_summary>';
    }
    return '$prompt\n\n<context_summary>\n$summaryText\n</context_summary>';
  }

  /// The largest output budget for a generated compaction summary.
  static const maxSummaryTokens = 4096;

  /// The instruction prefix used for compaction summary generation.
  static const _compactionInstruction =
      'You are summarizing the early portion of a session transcript so the '
      'conversation can continue within a compact context.\n\n'
      'Preserve, in dense factual plain text:\n'
      '- The user\'s goals and constraints.\n'
      '- Key decisions and their rationale.\n'
      '- The current task state and what remains undone.\n'
      '- Files, commands, and code touched, with their purposes.\n'
      '- Any unresolved issues or open questions.\n\n'
      'Do not summarize the recent messages that are kept in context verbatim.';

  /// Compacts the session context after a terminal turn when the context
  /// window is nearly exhausted, keeping the newest turns verbatim.
  Stream<AgentEvent> _compactContext({
    required Session session,
    required List<TimelineItem> timeline,
    required SessionContext? context,
    required ModelRef model,
    required TurnId turnId,
    required TokenUsage latestUsage,
    required int Function() nextSequence,
    bool enforceThreshold = true,
    String? instruction,
    CancellationToken? cancellation,
  }) async* {
    // Cancellation stops all trailing background work, including compaction.
    if (cancellation?.isCancelled == true) {
      return;
    }
    final kept = _keptWindow(timeline, keptRecentTurns);
    if (!enforceThreshold &&
        timeline.map((item) => item.turnId).toSet().length <= 1) {
      return;
    }
    // A long single turn can fill the context before a second turn exists.
    // Keep the newest item as the minimum live context and compact the prefix.
    final boundaryIndex = timeline.length - kept.length - 1 < 0
        ? (timeline.length > 1 ? 0 : -1)
        : timeline.length - kept.length - 1;
    if (boundaryIndex < 0) {
      return;
    }
    final boundary = timeline[boundaryIndex];
    final ModelDescriptor descriptor;
    try {
      descriptor = await provider.describe(model);
    } catch (_) {
      return;
    }
    if (descriptor.contextWindow <= 0) {
      return;
    }
    final tokens = latestUsage.inputTokens > 0
        ? latestUsage.inputTokens
        : estimateTokens(
            '${context == null ? '' : _systemPrompt(session, context)}\n'
            '${_renderTimeline(timeline)}',
          );
    if (enforceThreshold &&
        tokens < descriptor.contextWindow * compactionThreshold) {
      return;
    }
    yield CompactionStarted(
      sessionId: session.id,
      turnId: turnId,
      sequence: nextSequence(),
      occurredAt: _now().toUtc(),
    );
    try {
      final compacted = timeline.sublist(0, boundaryIndex + 1);
      final summary = await _generateSummary(
        session: session,
        compacted: compacted,
        model: model,
        turnId: turnId,
        instruction: instruction,
        cancellation: cancellation,
      );
      final checkpoint = CompactionCheckpoint(
        sessionId: session.id,
        compactedThroughSequence: boundary.sequence,
        summary: summary,
        keptRecentMessages: kept.length,
        inputTokensBefore: tokens,
        inputTokensAfter:
            estimateTokens(summary) + estimateTokens(_renderTimeline(kept)),
        createdAt: _now().toUtc(),
      );
      await store.saveCompaction(session.id, checkpoint);
      yield CompactionFinished(
        sessionId: session.id,
        turnId: turnId,
        sequence: nextSequence(),
        occurredAt: _now().toUtc(),
        checkpoint: checkpoint,
      );
    } on TurnCancelledException {
      rethrow;
    } catch (error) {
      yield CompactionFailed(
        sessionId: session.id,
        turnId: turnId,
        sequence: nextSequence(),
        occurredAt: _now().toUtc(),
        message: _safeErrorMessage('Context compaction failed', error),
      );
    }
  }

  /// Generates a compaction summary over the compacted items with one model
  /// call, chaining the previous checkpoint summary when present.
  Future<String> _generateSummary({
    required Session session,
    required List<TimelineItem> compacted,
    required ModelRef model,
    required TurnId turnId,
    String? instruction,
    CancellationToken? cancellation,
  }) async {
    final buffer = StringBuffer(_compactionInstruction);
    final instructionText = instruction?.trim();
    if (instructionText != null && instructionText.isNotEmpty) {
      buffer.write('\n\nAdditional user instruction:\n$instructionText');
    }
    final previous = session.compaction?.summary.trim();
    if (previous != null && previous.isNotEmpty) {
      buffer.write('\n\n<previous_summary>\n$previous\n</previous_summary>');
    }
    buffer.write(
      '\n\n<transcript>\n${_renderTimeline(compacted)}\n</transcript>',
    );
    final request = ModelRequest(
      sessionId: session.id,
      turnId: turnId,
      model: model,
      messages: [
        ModelMessage(
          role: ModelMessageRole.user,
          content: [TextContent(buffer.toString())],
        ),
      ],
      maxOutputTokens: maxSummaryTokens,
      cancellation: cancellation,
    );
    ModelResponse? completed;
    await for (final event in provider.stream(request)) {
      cancellation?.throwIfCancelled();
      switch (event) {
        case TextDeltaEvent() || ReasoningDeltaEvent():
          break;
        case ModelCompletedEvent(:final response):
          if (completed != null) {
            throw StateError('model stream completed more than once');
          }
          completed = response;
        case ModelFailedEvent(:final error, :final stackTrace):
          Error.throwWithStackTrace(error, stackTrace);
      }
    }
    if (completed == null) {
      throw StateError('model stream ended without response');
    }
    final summary = textFromContent(completed.content).trim();
    if (summary.isEmpty) {
      throw StateError('compaction summary is empty');
    }
    return summary;
  }

  /// Renders timeline items as a transcript for summary generation.
  static String _renderTimeline(List<TimelineItem> items) {
    final buffer = StringBuffer();
    for (final item in items) {
      switch (item) {
        case UserMessageItem(:final content):
          buffer.writeln(
            '<user>\n${_escapeXml(textFromContent(content))}\n</user>',
          );
        case AssistantMessageItem(:final content):
          buffer.writeln(
            '<assistant>\n${_escapeXml(textFromContent(content))}\n</assistant>',
          );
        case ToolCallItem(:final call):
          buffer
            ..write('<tool_call name="${_escapeXml(call.name)}" arguments="')
            ..write(_escapeXml(jsonEncode(call.arguments)))
            ..writeln('"/>');
        case ToolResultItem(:final content, :final isError):
          final rendered = _truncateToolResult(content);
          buffer
            ..writeln('<tool_result error="$isError">')
            ..writeln(_escapeXml(rendered))
            ..writeln('</tool_result>');
      }
    }
    return buffer.toString().trimRight();
  }

  /// Bounds tool output included in model-facing transcripts to avoid one
  /// command consuming the complete compaction request budget.
  static String _truncateToolResult(String content) {
    const limit = 4096;
    if (content.length <= limit) return content;
    final head = limit ~/ 2;
    final tail = limit - head;
    return '${content.substring(0, head)}\n...[tool result truncated; original length ${content.length}]...\n${content.substring(content.length - tail)}';
  }

  /// Returns the newest [keptRecentTurns] whole turns from [timeline].
  static List<TimelineItem> _keptWindow(
    List<TimelineItem> timeline,
    int keptRecentTurns,
  ) {
    if (timeline.isEmpty || keptRecentTurns <= 0) {
      return const [];
    }
    final keptTurns = <TurnId>{};
    for (
      var i = timeline.length - 1;
      i >= 0 && keptTurns.length < keptRecentTurns;
      i--
    ) {
      keptTurns.add(timeline[i].turnId);
    }
    return timeline.where((item) => keptTurns.contains(item.turnId)).toList();
  }

  /// Estimates token count from text using the common four-characters-per-
  /// token ratio; used when the provider does not report usage.
  static int estimateTokens(String text) {
    var cjk = 0;
    var other = 0;
    for (final rune in text.runes) {
      final isCjk =
          (rune >= 0x4e00 && rune <= 0x9fff) ||
          (rune >= 0x3400 && rune <= 0x4dbf) ||
          (rune >= 0xac00 && rune <= 0xd7af) ||
          (rune >= 0x3040 && rune <= 0x30ff);
      if (isCjk) {
        cjk++;
      } else {
        other++;
      }
    }
    return cjk + (other / 4).ceil();
  }

  Future<ToolResult> _executeTool({
    required Session session,
    required Turn turn,
    required ToolCall call,
    required CancellationToken cancellation,
  }) async {
    try {
      return await tools.execute(
        ToolContext(
          sessionId: session.id,
          turnId: turn.id,
          workingDirectory: session.workingDirectory,
          additionalDirectories: session.additionalDirectories,
          cancellation: cancellation,
        ),
        call,
      );
    } catch (error) {
      return ToolResult(
        content: _safeErrorMessage('Tool execution failed', error),
        isError: true,
      );
    }
  }

  static String _safeErrorMessage(String prefix, Object error) {
    if (error is SafeMessageException) {
      return '$prefix (${error.runtimeType}): ${error.safeMessage}';
    }
    return '$prefix (${error.runtimeType})';
  }

  /// The maximum total size of skill instructions injected into one turn.
  static const maxSelectedSkillBytes = 64 * 1024;

  /// Returns the cached session context, or null when the builder fails.
  SessionContext? _contextOrNull(String workingDirectory) {
    try {
      return _contextFor(workingDirectory);
    } catch (_) {
      return null;
    }
  }

  /// The cached session context for a working directory.
  SessionContext _contextFor(String workingDirectory) =>
      _sessionContexts.putIfAbsent(
        workingDirectory,
        () => sessionContextBuilder(workingDirectory),
      );

  /// Renders explicitly selected skills as non-persistent user context
  /// messages for the current turn, in first-selection order.
  List<ModelMessage> _skillMessages(List<String> names, SkillCatalog skills) {
    if (names.isEmpty) {
      return const [];
    }
    final result = <ModelMessage>[];
    final seen = <String>{};
    var total = 0;
    for (final name in names) {
      if (!seen.add(name)) {
        continue;
      }
      final skill = skills.lookup(name);
      if (skill == null) {
        continue;
      }
      total += skill.content.length;
      if (total > maxSelectedSkillBytes) {
        throw StateError('selected skill instructions exceed 64 KiB');
      }
      result.add(
        ModelMessage(
          role: ModelMessageRole.user,
          content: [TextContent(_skillContext(skill))],
        ),
      );
    }
    return result;
  }

  /// Wraps the full SKILL.md content in XML instruction tags.
  ///
  /// Only the metadata is escaped; the body is injected verbatim so the model
  /// reads the raw markdown, matching the instruction-file injection pattern.
  static String _skillContext(Skill skill) =>
      '<skill>\n<name>${_escapeXml(skill.name)}</name>\n'
      '<path>${_escapeXml(skill.path)}</path>\n'
      '${skill.content.trimRight()}\n</skill>';

  /// Escapes XML-significant characters in skill metadata and transcripts.
  static String _escapeXml(String value) => value
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;');

  /// Resolves the model descriptor for input-capability filtering.
  ///
  /// Returns null when the provider cannot describe the model; the provider's
  /// own request validation then rejects unsupported content.
  Future<ModelDescriptor?> _modelCapabilities(ModelRef model) async {
    try {
      return await provider.describe(model);
    } catch (_) {
      return null;
    }
  }

  /// Replaces image content with a placeholder when the active model cannot
  /// accept images, keeping the conversation usable after a model switch.
  ///
  /// Returns the original list when no filtering is needed.
  static List<ModelMessage> _applyInputCapabilities(
    List<ModelMessage> messages,
    ModelDescriptor? descriptor,
  ) {
    if (descriptor == null ||
        descriptor.inputCapabilities.contains(ModelInputCapability.image)) {
      return messages;
    }
    final filtered = <ModelMessage>[];
    var changed = false;
    for (final message in messages) {
      if (!message.content.any((part) => part is ImageContent)) {
        filtered.add(message);
        continue;
      }
      changed = true;
      filtered.add(
        ModelMessage(
          role: message.role,
          content: [
            for (final part in message.content)
              if (part is ImageContent)
                const TextContent(_omittedImagePlaceholder)
              else
                part,
          ],
          toolCalls: message.toolCalls,
          toolCallId: message.toolCallId,
          toolOutput: message.toolOutput,
          continuation: message.continuation,
        ),
      );
    }
    return changed ? filtered : messages;
  }

  /// Placeholder text replacing image content when the active model cannot
  /// accept images.
  static const _omittedImagePlaceholder =
      '[image omitted: current model does not support image input]';

  static List<ModelMessage> _projectTimeline(
    List<TimelineItem> items,
    List<ModelCheckpoint> checkpoints, {
    CompactionCheckpoint? compaction,
  }) {
    final continuations = {
      for (final checkpoint in checkpoints)
        checkpoint.timelineItemId: checkpoint.continuation,
    };
    final result = <ModelMessage>[];
    final visibleItems = compaction == null || compaction.summary.trim().isEmpty
        ? items
        : items.where(
            (item) => item.sequence > compaction.compactedThroughSequence,
          );
    for (final item in visibleItems) {
      switch (item) {
        case UserMessageItem(:final content):
          result.add(
            ModelMessage(role: ModelMessageRole.user, content: content),
          );
        case AssistantMessageItem(:final id, :final content):
          result.add(
            ModelMessage(
              role: ModelMessageRole.assistant,
              content: content,
              continuation: continuations[id],
            ),
          );
        case ToolCallItem(:final call):
          if (result.isNotEmpty &&
              result.last.role == ModelMessageRole.assistant) {
            final previous = result.removeLast();
            result.add(
              ModelMessage(
                role: ModelMessageRole.assistant,
                content: previous.content,
                toolCalls: [...previous.toolCalls, call],
                continuation: previous.continuation,
              ),
            );
          } else {
            result.add(
              ModelMessage(role: ModelMessageRole.assistant, toolCalls: [call]),
            );
          }
        case ToolResultItem(:final callId, :final content):
          result.add(
            ModelMessage(
              role: ModelMessageRole.tool,
              toolCallId: callId,
              toolOutput: content,
            ),
          );
      }
    }
    return List<ModelMessage>.unmodifiable(_dropOrphanToolCalls(result));
  }

  /// Removes tool calls that never received a tool result.
  ///
  /// A cancelled or interrupted turn can persist a tool call without its
  /// result; strict chat-completions providers reject such a message because
  /// every `tool_calls` entry must be answered by a tool message.
  static List<ModelMessage> _dropOrphanToolCalls(List<ModelMessage> messages) {
    final toolResultIds = <ToolCallId>{
      for (final message in messages)
        if (message.role == ModelMessageRole.tool && message.toolCallId != null)
          message.toolCallId!,
    };
    final cleaned = <ModelMessage>[];
    for (final message in messages) {
      if (message.role != ModelMessageRole.assistant ||
          message.toolCalls.isEmpty) {
        cleaned.add(message);
        continue;
      }
      final paired = [
        for (final call in message.toolCalls)
          if (toolResultIds.contains(call.id)) call,
      ];
      if (paired.length == message.toolCalls.length) {
        cleaned.add(message);
        continue;
      }
      if (paired.isNotEmpty || message.content.isNotEmpty) {
        cleaned.add(
          ModelMessage(
            role: message.role,
            content: message.content,
            toolCalls: paired,
            continuation: message.continuation,
          ),
        );
      }
    }
    return cleaned;
  }

  Turn _terminalTurn(
    Turn turn,
    TurnStatus status,
    TokenUsage usage, {
    TurnFailure? failure,
    String? cancelReason,
  }) => Turn(
    id: turn.id,
    sessionId: turn.sessionId,
    status: status,
    startedAt: turn.startedAt,
    completedAt: _now().toUtc(),
    model: turn.model,
    reasoningEffort: turn.reasoningEffort,
    usage: usage,
    failure: failure,
    cancelReason: cancelReason,
  );

  /// Persists a terminal turn, tolerating a session that was deleted while
  /// the turn was running: the terminal outcome still stands for the caller.
  Future<void> _finishTurnToleratingDeletion(
    SessionId sessionId,
    Turn terminal,
  ) async {
    try {
      await store.finishTurn(sessionId, terminal);
    } on SessionNotFoundException {
      // The session and its rows were deleted mid-turn.
    }
  }

  TurnFinished _finishedEvent(
    TurnOutcome outcome,
    SessionId sessionId,
    TurnId turnId,
    int sequence,
  ) => TurnFinished(
    sessionId: sessionId,
    turnId: turnId,
    sequence: sequence,
    occurredAt: _now().toUtc(),
    outcome: outcome,
  );

  static int _nextSequence(
    List<TimelineItem> items, {
    CompactionCheckpoint? compaction,
  }) => items.isNotEmpty
      ? items.last.sequence + 1
      : (compaction?.compactedThroughSequence ?? -1) + 1;

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

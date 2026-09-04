import '../domain/content.dart';
import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/session_context.dart';
import '../domain/timeline.dart';
import '../domain/turn.dart';
import '../domain/usage.dart';
import '../ports/cancellation.dart';
import '../ports/id_generator.dart';
import '../ports/logger.dart';
import '../ports/model_provider.dart';
import '../ports/session_store.dart';
import '../ports/tool_registry.dart';
import 'context_compactor.dart';
import 'error_summary.dart';
import 'model_request_composer.dart';

/// Executes local agent turns and manual compaction against loaded state.
///
/// Owns the single model/tool loop: every tool call is paired with a result,
/// emitted events preserve occurrence order, and every durable boundary is
/// persisted before the matching event is emitted. Locking and context
/// caching stay with the composing facade.
final class TurnExecutor {
  /// Creates an executor with injected ports and loop configuration.
  TurnExecutor({
    required this.store,
    required this.provider,
    required this.tools,
    required this.ids,
    required this.logger,
    required this.defaultModel,
    required this.compactor,
    required this.sessionContextOf,
    required this.systemPromptBuilder,
    DateTime Function()? now,
    this.maxSteps = 20,
    this.maxOutputTokens = 0,
    this.temperature,
  }) : _now = now ?? DateTime.now;

  /// The session persistence adapter.
  final SessionStore store;

  /// The model provider adapter.
  final ModelProvider provider;

  /// The registered local tools.
  final ToolRegistry tools;

  /// The ID generator used for new records.
  final IdGenerator ids;

  /// Structured diagnostic logger.
  final AtlasLogger logger;

  /// The model used when a turn does not provide an override.
  final ModelRef defaultModel;

  /// The compactor invoked before model requests, after terminal turns, and
  /// by manual compaction.
  final ContextCompactor compactor;

  /// Resolves the session context for a working directory; may throw.
  final SessionContext Function(String workingDirectory) sessionContextOf;

  /// Builds the system prompt for a session and turn.
  final String Function(SessionContext context) systemPromptBuilder;

  /// Maximum model/tool steps for one turn.
  final int maxSteps;

  /// Model output token limit.
  final int maxOutputTokens;

  /// Optional model temperature.
  final double? temperature;

  final DateTime Function() _now;

  /// Executes one turn and emits events in their exact occurrence order.
  Stream<AgentEvent> run(TurnRequest request) async* {
    final cancellation = request.cancellation ?? CancellationToken();
    final now = _now().toUtc();
    final model = request.model ?? defaultModel;
    final loaded = await _loadOrCreateSession(request, now);
    var session = loaded.session.title.isEmpty
        ? _withGeneratedTitle(loaded.session, request.content)
        : loaded.session;
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
    // Text deltas received by the in-flight model stream; flushed as an
    // aborted assistant item when cancellation interrupts the stream.
    final partialText = StringBuffer();
    try {
      final context = sessionContextOf(session.workingDirectory);
      final selectedSkillMessages = ModelRequestComposer.skillMessages(
        request.skills,
        context.skills,
      );
      // Resolved once per turn so historical images can be omitted for
      // models that cannot accept them; null falls back to provider-side
      // request validation.
      final capabilities = await _modelCapabilities(model);
      var compactedBeforeRequest = false;
      for (var step = 0; step < maxSteps; step++) {
        cancellation.throwIfCancelled();
        final compactionEvents = compactedBeforeRequest
            ? const <AgentEvent>[]
            : await _compactBeforeRequest(
                session: session,
                timeline: timeline,
                model: model,
                turnId: turnId,
                latestUsage: latestUsage,
                cancellation: cancellation,
                nextSequence: () => eventSequence++,
              );
        for (final event in compactionEvents) {
          if (event case CompactionFinished(:final checkpoint)) {
            session = _withCompaction(session, checkpoint);
            timeline.removeWhere(
              (item) => item.sequence <= checkpoint.compactedThroughSequence,
            );
            compactedBeforeRequest = true;
          }
          yield event;
        }
        ModelResponse? response;
        final modelRequest = ModelRequest(
          sessionId: session.id,
          turnId: turn.id,
          model: model,
          messages: ModelRequestComposer.applyInputCapabilities([
            ...selectedSkillMessages,
            ...ModelRequestComposer.projectTimeline(
              timeline,
              modelCheckpoints,
              compaction: session.compaction,
            ),
          ], capabilities),
          systemPrompt: systemPromptBuilder(context),
          tools: tools.descriptors,
          reasoningEffort: request.reasoningEffort,
          maxOutputTokens: maxOutputTokens,
          temperature: temperature,
          cancellation: cancellation,
        );
        partialText.clear();
        await for (final event in provider.stream(modelRequest)) {
          cancellation.throwIfCancelled();
          switch (event) {
            case TextDeltaEvent(:final delta):
              partialText.write(delta);
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
        // The step persists in full below; the partial buffer is stale now.
        partialText.clear();

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
          usage: latestUsage,
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
          yield* _compactorAfterTurn(
            session: session,
            timeline: timeline,
            context: context,
            model: model,
            turnId: turnId,
            latestUsage: latestUsage,
            cancellation: cancellation,
            nextSequence: () => eventSequence++,
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
      final partialItem = await _flushAbortedPartial(
        sessionId: session.id,
        turnId: turnId,
        timeline: timeline,
        model: model,
        latestUsage: latestUsage,
        partialText: partialText,
      );
      if (partialItem != null) {
        yield ModelResponseReceived(
          sessionId: session.id,
          turnId: turnId,
          sequence: eventSequence++,
          occurredAt: _now().toUtc(),
          assistantMessage: partialItem,
          toolCalls: const [],
        );
      }
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
      yield* _compactorAfterTurn(
        session: session,
        timeline: timeline,
        context: _contextOrNull(session.workingDirectory),
        model: model,
        turnId: turnId,
        latestUsage: latestUsage,
        cancellation: cancellation,
        nextSequence: () => eventSequence++,
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
          message: safeErrorMessage('Turn failed', error),
          providerDetail: providerDetail(error),
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
            message: safeErrorMessage('Turn failed', error),
            providerDetail: providerDetail(error),
          ),
        ),
      );
      yield _finishedEvent(outcome, session.id, turnId, eventSequence++);
      yield* _compactorAfterTurn(
        session: session,
        timeline: timeline,
        context: _contextOrNull(session.workingDirectory),
        model: model,
        turnId: turnId,
        latestUsage: latestUsage,
        cancellation: cancellation,
        nextSequence: () => eventSequence++,
      );
      Error.throwWithStackTrace(error, stackTrace);
    }
  }

  /// Runs a threshold-less compaction over the latest persisted state.
  ///
  /// Uses the model and turn of the snapshot's most recent turn so the
  /// emitted events stay attached to the session's last execution.
  Stream<AgentEvent> compact(
    SessionSnapshot snapshot, {
    String? instruction,
    CancellationToken? cancellation,
  }) async* {
    final turns = snapshot.turns;
    if (turns.isEmpty) {
      return;
    }
    final lastTurn = turns.last;
    var eventSequence = 0;
    yield* compactor.compact(
      CompactionJob(
        session: snapshot.session,
        timeline: snapshot.timeline,
        systemPrompt: _sessionPrompt(snapshot.session),
        model: lastTurn.model ?? defaultModel,
        turnId: lastTurn.id,
        latestUsage: lastTurn.usage,
        enforceThreshold: false,
        instruction: instruction,
        cancellation: cancellation,
        nextSequence: () => eventSequence++,
      ),
    );
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

  /// Derives the initial display title from the first user message.
  Session _withGeneratedTitle(Session session, List<ContentPart> content) {
    final text = textFromContent(content).trim();
    if (text.isEmpty) return session;
    final firstLine = text.split('\n').first.trim();
    final title = String.fromCharCodes(firstLine.runes.take(80));
    return Session(
      id: session.id,
      title: title,
      workingDirectory: session.workingDirectory,
      additionalDirectories: session.additionalDirectories,
      createdAt: session.createdAt,
      updatedAt: session.updatedAt,
      compaction: session.compaction,
      lastUsage: session.lastUsage,
    );
  }

  /// The current system prompt for [session], or '' when its cached context
  /// is unavailable; used for compaction token estimates. The compaction
  /// summary is projected as the first user message, not appended here.
  String _sessionPrompt(Session session) {
    final context = _contextOrNull(session.workingDirectory);
    return context == null ? '' : systemPromptBuilder(context);
  }

  /// Returns the session context, or null when resolution fails.
  SessionContext? _contextOrNull(String workingDirectory) {
    try {
      return sessionContextOf(workingDirectory);
    } catch (_) {
      return null;
    }
  }

  /// Attempts threshold compaction before the next model request.
  Future<List<AgentEvent>> _compactBeforeRequest({
    required Session session,
    required List<TimelineItem> timeline,
    required ModelRef model,
    required TurnId turnId,
    required TokenUsage latestUsage,
    required CancellationToken cancellation,
    required int Function() nextSequence,
  }) {
    if (!compactor.usesTokenWindow) {
      return Future.value(const <AgentEvent>[]);
    }
    return compactor
        .compact(
          CompactionJob(
            session: session,
            timeline: _visibleTimeline(timeline, session.compaction),
            systemPrompt: _sessionPrompt(session),
            model: model,
            turnId: turnId,
            latestUsage: latestUsage,
            nextSequence: nextSequence,
            cancellation: cancellation,
          ),
        )
        .toList();
  }

  /// Builds the post-turn compaction job for [session].
  Stream<AgentEvent> _compactorAfterTurn({
    required Session session,
    required List<TimelineItem> timeline,
    required SessionContext? context,
    required ModelRef model,
    required TurnId turnId,
    required TokenUsage latestUsage,
    required int Function() nextSequence,
    CancellationToken? cancellation,
  }) => compactor.compact(
    CompactionJob(
      session: session,
      timeline: _visibleTimeline(timeline, session.compaction),
      systemPrompt: context == null ? '' : systemPromptBuilder(context),
      model: model,
      turnId: turnId,
      latestUsage: latestUsage,
      nextSequence: nextSequence,
      cancellation: cancellation,
    ),
  );

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
        content: safeErrorMessage('Tool execution failed', error),
        isError: true,
      );
    }
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

  /// Persists text received by a stream that cancellation interrupted as an
  /// aborted assistant item; returns null when the stream had delivered no
  /// text or the append failed, so the terminal turn state is always reached.
  Future<AssistantMessageItem?> _flushAbortedPartial({
    required SessionId sessionId,
    required TurnId turnId,
    required List<TimelineItem> timeline,
    required ModelRef model,
    required TokenUsage latestUsage,
    required StringBuffer partialText,
  }) async {
    if (partialText.isEmpty) return null;
    final item = AssistantMessageItem(
      id: ids.timelineItemId(),
      sessionId: sessionId,
      turnId: turnId,
      sequence: _nextSequence(timeline),
      occurredAt: _now().toUtc(),
      content: List<ContentPart>.unmodifiable([
        TextContent(partialText.toString()),
      ]),
      model: model,
      stopReason: StopReason.aborted,
      usage: latestUsage,
    );
    try {
      await store.appendModelStep(
        sessionId,
        PersistedModelStep(assistantMessage: item, toolCalls: const []),
      );
    } on Exception catch (error) {
      // The partial is best-effort enrichment of the cancellation path; a
      // flush failure (for example a concurrently deleted session) must not
      // skip the terminal turn state below.
      logger.log(
        LogEvent(
          level: LogLevel.warn,
          code: 'turn.partial_flush_failed',
          message: safeErrorMessage('Partial flush failed', error),
          sessionId: sessionId,
          turnId: turnId,
          occurredAt: _now().toUtc(),
        ),
      );
      return null;
    }
    partialText.clear();
    timeline.add(item);
    return item;
  }

  /// Persists a terminal turn, tolerating a session that was deleted while
  /// the turn was running: the terminal outcome still stands for the caller.
  static List<TimelineItem> _visibleTimeline(
    List<TimelineItem> timeline,
    CompactionCheckpoint? compaction,
  ) {
    if (compaction == null || compaction.summary.trim().isEmpty) {
      return timeline;
    }
    return timeline
        .where((item) => item.sequence > compaction.compactedThroughSequence)
        .toList();
  }

  Session _withCompaction(Session session, CompactionCheckpoint checkpoint) =>
      Session(
        id: session.id,
        title: session.title,
        workingDirectory: session.workingDirectory,
        additionalDirectories: session.additionalDirectories,
        createdAt: session.createdAt,
        updatedAt: session.updatedAt,
        compaction: checkpoint,
        lastUsage: session.lastUsage,
        model: session.model,
        reasoningEffort: session.reasoningEffort,
      );

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
}

import 'dart:async';

import '../domain/content.dart';
import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/timeline.dart';
import '../domain/turn.dart';
import '../domain/usage.dart';
import '../ports/cancellation.dart';
import '../ports/id_generator.dart';
import '../ports/model_provider.dart';
import '../ports/session_store.dart';
import '../ports/tool_registry.dart';

/// Executes model turns and persists every durable boundary through ports.
final class AgentRuntime {
  /// Creates an agent runtime with injected model, tool, and storage adapters.
  AgentRuntime({
    required this.store,
    required this.provider,
    required this.tools,
    required this.ids,
    required this.defaultModel,
    DateTime Function()? now,
    this.systemPromptBuilder = _emptySystemPrompt,
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

  /// The model used when a turn does not provide an override.
  final ModelRef defaultModel;

  /// Builds the system prompt for a session and turn.
  final String Function(SessionId sessionId, TurnRequest request)
  systemPromptBuilder;

  /// Maximum model/tool steps for one turn.
  final int maxSteps;

  /// Model output token limit.
  final int maxOutputTokens;

  /// Optional model temperature.
  final double? temperature;

  final DateTime Function() _now;
  final Map<SessionId, Future<void>> _sessionTails = {};

  /// Executes one turn and emits events in their exact occurrence order.
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
    final sequence = _nextSequence(loaded.timeline);
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
      for (var step = 0; step < maxSteps; step++) {
        cancellation.throwIfCancelled();
        ModelResponse? response;
        final modelRequest = ModelRequest(
          sessionId: session.id,
          turnId: turn.id,
          model: model,
          messages: _projectTimeline(
            timeline,
            modelCheckpoints,
            compaction: session.compaction,
          ),
          systemPrompt: _systemPrompt(session, request),
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
          response: completedResponse,
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
          );
          await store.finishTurn(
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
          return;
        }

        for (final callItem in calls) {
          cancellation.throwIfCancelled();
          yield ToolStarted(
            sessionId: session.id,
            turnId: turnId,
            sequence: eventSequence++,
            occurredAt: _now().toUtc(),
            call: callItem,
          );
          final result = await _executeTool(
            session: session,
            turn: turn,
            request: request,
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
      await store.finishTurn(
        session.id,
        _terminalTurn(
          turn,
          TurnStatus.cancelled,
          latestUsage,
          cancelReason: error.toString(),
        ),
      );
      yield _finishedEvent(outcome, session.id, turnId, eventSequence++);
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
      await store.finishTurn(
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
      Error.throwWithStackTrace(error, stackTrace);
    }
  }

  Future<_LoadedSession> _loadOrCreateSession(
    TurnRequest request,
    DateTime now,
  ) async {
    if (request.sessionId != null) {
      final snapshot = await store.loadSession(request.sessionId!);
      final current = snapshot.session;
      final session = Session(
        id: current.id,
        title: current.title,
        workingDirectory: request.workingDirectory ?? current.workingDirectory,
        additionalDirectories: List<String>.unmodifiable(
          request.additionalDirectories ?? current.additionalDirectories,
        ),
        createdAt: current.createdAt,
        updatedAt: current.updatedAt,
        compaction: current.compaction,
        lastUsage: current.lastUsage,
      );
      return _LoadedSession.fromSnapshot(snapshot, session);
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
    return _LoadedSession.newSession(session);
  }

  String _systemPrompt(Session session, TurnRequest request) {
    final prompt = systemPromptBuilder(session.id, request);
    final summary = session.compaction?.summary.trim();
    if (summary == null || summary.isEmpty) {
      return prompt;
    }
    if (prompt.trim().isEmpty) {
      return '<context_summary>\n$summary\n</context_summary>';
    }
    return '$prompt\n\n<context_summary>\n$summary\n</context_summary>';
  }

  Future<ToolResult> _executeTool({
    required Session session,
    required Turn turn,
    required TurnRequest request,
    required ToolCall call,
    required CancellationToken cancellation,
  }) async {
    try {
      return await tools.execute(
        ToolContext(
          sessionId: session.id,
          turnId: turn.id,
          workingDirectory:
              request.workingDirectory ?? session.workingDirectory,
          additionalDirectories:
              request.additionalDirectories ?? session.additionalDirectories,
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

  static String _safeErrorMessage(String prefix, Object error) =>
      '$prefix (${error.runtimeType})';

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
        case PlanUpdatedItem():
        case CompactionItem():
          break;
      }
    }
    return List<ModelMessage>.unmodifiable(result);
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

  static int _nextSequence(List<TimelineItem> items) =>
      items.isEmpty ? 0 : items.last.sequence + 1;

  static String _emptySystemPrompt(SessionId sessionId, TurnRequest request) =>
      '';

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

final class _LoadedSession {
  _LoadedSession.fromSnapshot(SessionSnapshot snapshot, this.session)
    : snapshot = snapshot,
      assert(session.id == snapshot.session.id);

  _LoadedSession.newSession(this.session) : snapshot = null;

  final SessionSnapshot? snapshot;
  final Session session;

  List<TimelineItem> get timeline => snapshot?.timeline ?? const [];

  List<ModelCheckpoint> get modelCheckpoints =>
      snapshot?.modelCheckpoints ?? const [];
}

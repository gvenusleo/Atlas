import 'ids.dart';
import 'timeline.dart';
import 'turn.dart';
import 'usage.dart';

/// A runtime event emitted while executing a turn.
sealed class AgentEvent {
  /// Creates an agent event.
  const AgentEvent({
    required this.sessionId,
    required this.turnId,
    required this.sequence,
    required this.occurredAt,
  });

  /// The owning session.
  final SessionId sessionId;

  /// The current turn.
  final TurnId turnId;

  /// The event sequence within the execution.
  final int sequence;

  /// The UTC event time.
  final DateTime occurredAt;
}

/// Indicates that a turn has entered the runtime.
final class TurnStarted extends AgentEvent {
  /// Creates a turn-started event.
  const TurnStarted({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.userMessage,
  });

  /// The persisted user message.
  final UserMessageItem userMessage;
}

/// An incremental assistant text event.
final class ModelTextDelta extends AgentEvent {
  /// Creates a model text delta.
  const ModelTextDelta({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.delta,
  });

  /// The new text fragment.
  final String delta;
}

/// An incremental reasoning summary event.
final class ModelReasoningDelta extends AgentEvent {
  /// Creates a reasoning delta.
  const ModelReasoningDelta({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.delta,
  });

  /// The new reasoning fragment.
  final String delta;
}

/// Indicates that a model step completed and was persisted.
final class ModelResponseReceived extends AgentEvent {
  /// Creates a model response event.
  const ModelResponseReceived({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.assistantMessage,
    required this.toolCalls,
    this.usage = const TokenUsage(),
  });

  /// The persisted assistant item.
  final AssistantMessageItem assistantMessage;

  /// Persisted tool call items emitted by this response.
  final List<ToolCallItem> toolCalls;

  /// The provider usage of the completed model request backing this
  /// response; zero-valued when unknown so clients keep the previous figure.
  final TokenUsage usage;
}

/// Indicates that a tool is about to execute.
final class ToolStarted extends AgentEvent {
  /// Creates a tool-started event.
  const ToolStarted({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.call,
  });

  /// The requested tool call.
  final ToolCallItem call;
}

/// Indicates that a tool produced a result.
final class ToolFinished extends AgentEvent {
  /// Creates a tool-finished event.
  const ToolFinished({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.result,
  });

  /// The persisted tool result.
  final ToolResultItem result;
}

/// One step of an agent plan.
final class PlanEntry {
  /// Creates a plan entry.
  const PlanEntry({
    required this.content,
    this.priority = 'medium',
    this.status = 'pending',
  });

  /// The step description.
  final String content;

  /// The step priority (`high`, `medium`, or `low`).
  final String priority;

  /// The step status (`pending`, `in_progress`, or `completed`).
  final String status;
}

/// Indicates that the agent replaced its plan for the current turn.
final class PlanUpdated extends AgentEvent {
  /// Creates a plan-updated event.
  const PlanUpdated({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.entries,
  });

  /// The complete plan, replacing any previous plan for this turn.
  final List<PlanEntry> entries;
}

/// Indicates that context compaction has started.
final class CompactionStarted extends AgentEvent {
  /// Creates a compaction-started event.
  const CompactionStarted({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
  });
}

/// Indicates that a context checkpoint was persisted.
final class CompactionFinished extends AgentEvent {
  /// Creates a compaction-finished event.
  const CompactionFinished({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.checkpoint,
  });

  /// The persisted compaction checkpoint.
  final CompactionCheckpoint checkpoint;
}

/// Indicates that compaction failed without affecting the turn outcome.
final class CompactionFailed extends AgentEvent {
  /// Creates a compaction-failed event.
  const CompactionFailed({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.message,
  });

  /// A safe, model-visible description of the failure.
  final String message;
}

/// Indicates that a turn completed successfully or with a terminal state.
final class TurnFinished extends AgentEvent {
  /// Creates a turn-finished event.
  const TurnFinished({
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.outcome,
  });

  /// The terminal outcome.
  final TurnOutcome outcome;
}

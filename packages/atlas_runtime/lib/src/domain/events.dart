import 'ids.dart';
import 'timeline.dart';
import 'turn.dart';
import 'usage.dart';
import '../ports/tool_registry.dart';

/// A runtime event emitted while executing a turn.
sealed class const AgentEvent({
  /// The owning session.
  required final SessionId sessionId,

  /// The current turn.
  required final TurnId turnId,

  /// The event sequence within the execution.
  required final int sequence,

  /// The UTC event time.
  required final DateTime occurredAt,
}) {
  /// Creates an agent event.
  this;
}

/// Indicates that a turn has entered the runtime.
final class const TurnStarted({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The persisted user message.
  required final UserMessageItem userMessage,
}) extends AgentEvent {
  /// Creates a turn-started event.
  this;
}

/// An incremental assistant text event.
final class const ModelTextDelta({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The new text fragment.
  required final String delta,
}) extends AgentEvent {
  /// Creates a model text delta.
  this;
}

/// An incremental reasoning summary event.
final class const ModelReasoningDelta({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The new reasoning fragment.
  required final String delta,
}) extends AgentEvent {
  /// Creates a reasoning delta.
  this;
}

/// Indicates that a model step completed and was persisted.
final class const ModelResponseReceived({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The persisted assistant item.
  required final AssistantMessageItem assistantMessage,

  /// Persisted tool call items emitted by this response.
  required final List<ToolCallItem> toolCalls,

  /// The provider usage of the completed model request backing this
  /// response; zero-valued when unknown so clients keep the previous figure.
  final TokenUsage usage = const TokenUsage(),
}) extends AgentEvent {
  /// Creates a model response event.
  this;
}

/// Indicates that the context occupancy changed without a turn completing.
///
/// Synthesized by protocol clients that receive usage outside a turn
/// boundary (ACP `session/update`), so presentation code can refresh live
/// usage figures while tools still run. The local runtime reports usage
/// through [ModelResponseReceived] instead.
final class const UsageUpdated({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The current context occupancy.
  required final TokenUsage usage,
}) extends AgentEvent {
  /// Creates a usage-updated event.
  this;
}

/// Indicates that a tool is about to execute.
final class const ToolStarted({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The requested tool call.
  required final ToolCallItem call,
}) extends AgentEvent {
  /// Creates a tool-started event.
  this;
}

/// Indicates that a tool produced a result.
final class const ToolFinished({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The persisted tool result.
  required final ToolResultItem result,
}) extends AgentEvent {
  /// Creates a tool-finished event.
  this;
}

/// Replaces a running tool's displayed output without changing its timeline.
final class const ToolOutputUpdated({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The invocation producing this output.
  required final ToolCallId callId,

  /// Bounded text and counters replacing the previous display state.
  required final ToolOutputSnapshot output,
}) extends AgentEvent {
  /// Creates an output update associated with one tool invocation.
  this;
}

/// One step of an agent plan.
final class const PlanEntry({
  /// The step description.
  required final String content,

  /// The step priority (`high`, `medium`, or `low`).
  final String priority = 'medium',

  /// The step status (`pending`, `in_progress`, or `completed`).
  final String status = 'pending',
}) {
  /// Creates a plan entry.
  this;
}

/// Indicates that the agent replaced its plan for the current turn.
final class const PlanUpdated({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The complete plan, replacing any previous plan for this turn.
  required final List<PlanEntry> entries,
}) extends AgentEvent {
  /// Creates a plan-updated event.
  this;
}

/// Indicates that context compaction has started.
final class const CompactionStarted({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,
}) extends AgentEvent {
  /// Creates a compaction-started event.
  this;
}

/// Indicates that a context checkpoint was persisted.
final class const CompactionFinished({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The persisted compaction checkpoint.
  required final CompactionCheckpoint checkpoint,
}) extends AgentEvent {
  /// Creates a compaction-finished event.
  this;
}

/// Indicates that compaction failed without affecting the turn outcome.
final class const CompactionFailed({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// A safe, model-visible description of the failure.
  required final String message,
}) extends AgentEvent {
  /// Creates a compaction-failed event.
  this;
}

/// Indicates that a turn completed successfully or with a terminal state.
final class const TurnFinished({
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The terminal outcome.
  required final TurnOutcome outcome,
}) extends AgentEvent {
  /// Creates a turn-finished event.
  this;
}

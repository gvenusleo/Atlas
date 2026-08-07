import 'ids.dart';
import 'model.dart';
import 'timeline.dart';
import 'turn.dart';

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
    required this.response,
    required this.assistantMessage,
    required this.toolCalls,
  });

  /// The provider-neutral response.
  final ModelResponse response;

  /// The persisted assistant item.
  final AssistantMessageItem assistantMessage;

  /// Persisted tool call items emitted by this response.
  final List<ToolCallItem> toolCalls;
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

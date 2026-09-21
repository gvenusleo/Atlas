import 'content.dart';
import 'ids.dart';
import 'model.dart';
import 'usage.dart';

/// A persisted item in a session's ordered timeline.
sealed class const TimelineItem({
  /// The item identifier.
  required final TimelineItemId id,

  /// The owning session.
  required final SessionId sessionId,

  /// The owning turn.
  required final TurnId turnId,

  /// The strict order inside the session.
  required final int sequence,

  /// The UTC time at which the item was appended.
  required final DateTime occurredAt,
}) {
  /// Creates a timeline item.
  this;
}

/// A user-submitted message.
final class const UserMessageItem({
  required super.id,
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The submitted content parts.
  required final List<ContentPart> content,
}) extends TimelineItem {
  /// Creates a user message item.
  this;
}

/// A completed assistant response.
final class const AssistantMessageItem({
  required super.id,
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The assistant content parts.
  required final List<ContentPart> content,

  /// The model that produced this response.
  required final ModelRef model,

  /// The provider stop reason.
  required final StopReason stopReason,

  /// Provider-neutral reasoning text produced before the response.
  final String reasoning = '',

  /// Token usage for the response.
  final TokenUsage usage = const TokenUsage(),
}) extends TimelineItem {
  /// Creates an assistant message item.
  this;
}

/// A tool call emitted by an assistant response.
final class const ToolCallItem({
  required super.id,
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The model-requested call.
  required final ToolCall call,
}) extends TimelineItem {
  /// Creates a tool call item.
  this;
}

/// A completed result for a tool call.
final class const ToolResultItem({
  required super.id,
  required super.sessionId,
  required super.turnId,
  required super.sequence,
  required super.occurredAt,

  /// The matching tool call identifier.
  required final ToolCallId callId,

  /// The tool output text.
  required final String content,

  /// Whether the tool failed.
  final bool isError = false,

  /// Structured tool result data.
  final JsonObject metadata = const <String, Object?>{},
}) extends TimelineItem {
  /// Creates a tool result item.
  this;
}

/// The value returned by a tool implementation before persistence.
final class const ToolResult({
  /// The tool output text.
  required final String content,

  /// Whether the tool failed.
  final bool isError = false,

  /// Structured tool result data.
  final JsonObject metadata = const <String, Object?>{},
}) {
  /// Creates a tool result.
  this;
}

/// A durable context compaction checkpoint.
final class const CompactionCheckpoint({
  /// The owning session.
  required final SessionId sessionId,

  /// The last timeline sequence represented by [summary].
  required final int compactedThroughSequence,

  /// The generated summary used for future model context.
  required final String summary,

  /// Timeline messages after the boundary that were kept verbatim.
  required final int keptRecentMessages,

  /// Input tokens before compaction.
  required final int inputTokensBefore,

  /// Input tokens after compaction.
  required final int inputTokensAfter,

  /// The UTC creation time.
  required final DateTime createdAt,
}) {
  /// Creates a compaction checkpoint.
  this;
}

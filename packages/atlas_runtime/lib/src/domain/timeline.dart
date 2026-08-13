import 'content.dart';
import 'ids.dart';
import 'model.dart';
import 'usage.dart';

/// A persisted item in a session's ordered timeline.
sealed class TimelineItem {
  /// Creates a timeline item.
  const TimelineItem({
    required this.id,
    required this.sessionId,
    required this.turnId,
    required this.sequence,
    required this.occurredAt,
  });

  /// The item identifier.
  final TimelineItemId id;

  /// The owning session.
  final SessionId sessionId;

  /// The owning turn.
  final TurnId turnId;

  /// The strict order inside the session.
  final int sequence;

  /// The UTC time at which the item was appended.
  final DateTime occurredAt;
}

/// A user-submitted message.
final class UserMessageItem extends TimelineItem {
  /// Creates a user message item.
  const UserMessageItem({
    required super.id,
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.content,
  });

  /// The submitted content parts.
  final List<ContentPart> content;
}

/// A completed assistant response.
final class AssistantMessageItem extends TimelineItem {
  /// Creates an assistant message item.
  const AssistantMessageItem({
    required super.id,
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.content,
    required this.model,
    required this.stopReason,
    this.usage = const TokenUsage(),
  });

  /// The assistant content parts.
  final List<ContentPart> content;

  /// The model that produced this response.
  final ModelRef model;

  /// The provider stop reason.
  final StopReason stopReason;

  /// Token usage for the response.
  final TokenUsage usage;
}

/// A tool call emitted by an assistant response.
final class ToolCallItem extends TimelineItem {
  /// Creates a tool call item.
  const ToolCallItem({
    required super.id,
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.call,
  });

  /// The model-requested call.
  final ToolCall call;
}

/// A completed result for a tool call.
final class ToolResultItem extends TimelineItem {
  /// Creates a tool result item.
  const ToolResultItem({
    required super.id,
    required super.sessionId,
    required super.turnId,
    required super.sequence,
    required super.occurredAt,
    required this.callId,
    required this.content,
    this.isError = false,
    this.metadata = const <String, Object?>{},
  });

  /// The matching tool call identifier.
  final ToolCallId callId;

  /// The tool output text.
  final String content;

  /// Whether the tool failed.
  final bool isError;

  /// Structured tool result data.
  final JsonObject metadata;
}

/// The value returned by a tool implementation before persistence.
final class ToolResult {
  /// Creates a tool result.
  const ToolResult({
    required this.content,
    this.isError = false,
    this.metadata = const <String, Object?>{},
  });

  /// The tool output text.
  final String content;

  /// Whether the tool failed.
  final bool isError;

  /// Structured tool result data.
  final JsonObject metadata;
}

/// A durable context compaction checkpoint.
final class CompactionCheckpoint {
  /// Creates a compaction checkpoint.
  const CompactionCheckpoint({
    required this.sessionId,
    required this.compactedThroughSequence,
    required this.summary,
    required this.keptRecentMessages,
    required this.inputTokensBefore,
    required this.inputTokensAfter,
    required this.createdAt,
  });

  /// The owning session.
  final SessionId sessionId;

  /// The last timeline sequence represented by [summary].
  final int compactedThroughSequence;

  /// The generated summary used for future model context.
  final String summary;

  /// Timeline messages after the boundary that were kept verbatim.
  final int keptRecentMessages;

  /// Input tokens before compaction.
  final int inputTokensBefore;

  /// Input tokens after compaction.
  final int inputTokensAfter;

  /// The UTC creation time.
  final DateTime createdAt;
}

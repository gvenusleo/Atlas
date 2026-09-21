import 'content.dart';

/// A presentation-safe conversation item without persistence identities.
sealed class const ConversationItem() {
  /// Creates a conversation item.
  this;
}

/// A user message reconstructed for display.
final class const ConversationUserMessage(
  /// Message content.
  final List<ContentPart> content,
) extends ConversationItem {
  /// Creates a user message.
  this;
}

/// An assistant message reconstructed for display.
final class const ConversationAssistantMessage(
  /// Message content.
  final List<ContentPart> content, {

  /// Provider-neutral reasoning text.
  final String reasoning = '',
}) extends ConversationItem {
  /// Creates an assistant message.
  this;
}

/// A displayed tool call.
final class const ConversationToolCall({
  /// Protocol-level call identity used only for display pairing.
  required final String callId,

  /// Tool name.
  required final String name,

  /// Tool arguments.
  required final Map<String, Object?> arguments,
}) extends ConversationItem {
  /// Creates a tool call.
  this;
}

/// A displayed tool result.
final class const ConversationToolResult({
  /// Call identity paired with [ConversationToolCall.callId].
  required final String callId,

  /// Rendered result content.
  required final String content,

  /// Whether the tool failed or was cancelled.
  final bool isError = false,
}) extends ConversationItem {
  /// Creates a tool result.
  this;
}

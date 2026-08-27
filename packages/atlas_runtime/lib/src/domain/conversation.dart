import 'content.dart';

/// A presentation-safe conversation item without persistence identities.
sealed class ConversationItem {
  /// Creates a conversation item.
  const ConversationItem();
}

/// A user message reconstructed for display.
final class ConversationUserMessage extends ConversationItem {
  /// Creates a user message.
  const ConversationUserMessage(this.content);

  /// Message content.
  final List<ContentPart> content;
}

/// An assistant message reconstructed for display.
final class ConversationAssistantMessage extends ConversationItem {
  /// Creates an assistant message.
  const ConversationAssistantMessage(this.content, {this.reasoning = ''});

  /// Message content.
  final List<ContentPart> content;

  /// Provider-neutral reasoning text.
  final String reasoning;
}

/// A displayed tool call.
final class ConversationToolCall extends ConversationItem {
  /// Creates a tool call.
  const ConversationToolCall({
    required this.callId,
    required this.name,
    required this.arguments,
  });

  /// Protocol-level call identity used only for display pairing.
  final String callId;

  /// Tool name.
  final String name;

  /// Tool arguments.
  final Map<String, Object?> arguments;
}

/// A displayed tool result.
final class ConversationToolResult extends ConversationItem {
  /// Creates a tool result.
  const ConversationToolResult({
    required this.callId,
    required this.content,
    this.isError = false,
  });

  /// Call identity paired with [ConversationToolCall.callId].
  final String callId;

  /// Rendered result content.
  final String content;

  /// Whether the tool failed or was cancelled.
  final bool isError;
}

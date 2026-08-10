/// Rendered message kinds in the chat transcript.
enum ChatMessageKind {
  /// The user's submitted text.
  user,

  /// Assistant text accumulated from model deltas.
  assistant,

  /// Model reasoning summary text.
  reasoning,

  /// A tool call and its result.
  tool,

  /// A turn-level failure.
  error,

  /// A local notice such as slash command help.
  system,
}

/// One rendered message in the chat transcript.
final class ChatMessage {
  /// Creates a chat message.
  const ChatMessage({
    required this.kind,
    required this.text,
    this.toolName,
    this.isError = false,
  });

  /// The message kind.
  final ChatMessageKind kind;

  /// The rendered text.
  final String text;

  /// The tool name for [ChatMessageKind.tool] messages.
  final String? toolName;

  /// Whether this message represents a failure.
  final bool isError;
}

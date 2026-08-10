import 'package:nocterm/nocterm.dart';

import 'chat_message.dart';
import 'prompt_line.dart';

/// Renders the transcript as a scrollable list.
final class MessageList extends StatelessComponent {
  /// Creates a message list.
  const MessageList({super.key, required this.messages});

  /// The messages to render, in occurrence order.
  final List<ChatMessage> messages;

  @override
  Component build(BuildContext context) {
    return ListView.builder(
      itemCount: messages.length,
      itemBuilder: (context, index) => _MessageRow(message: messages[index]),
    );
  }
}

/// One rendered transcript line.
final class _MessageRow extends StatelessComponent {
  const _MessageRow({required this.message});

  final ChatMessage message;

  @override
  Component build(BuildContext context) {
    return switch (message.kind) {
      ChatMessageKind.user => PromptLine(
        child: Text(
          message.text,
          style: const TextStyle(fontWeight: FontWeight.bold),
        ),
      ),
      ChatMessageKind.assistant => MarkdownText(
        message.text,
        styleSheet: MarkdownStyleSheet(),
      ),
      ChatMessageKind.reasoning => Text(
        message.text,
        style: TextStyle(color: Color.fromRGB(128, 128, 128)),
      ),
      ChatMessageKind.tool => Text(
        '⚙ ${message.toolName ?? 'tool'}: ${message.text}',
        style: TextStyle(
          color: message.isError
              ? Color.fromRGB(220, 50, 47)
              : Color.fromRGB(100, 149, 237),
        ),
      ),
      ChatMessageKind.error => Text(
        '✗ ${message.text}',
        style: TextStyle(color: Color.fromRGB(220, 50, 47)),
      ),
    };
  }
}

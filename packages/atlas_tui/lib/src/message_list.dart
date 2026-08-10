import 'package:nocterm/nocterm.dart';

import 'chat_message.dart';
import 'prompt_line.dart';

/// Bold text style shared by headings and emphasis.
const _boldStyle = TextStyle(fontWeight: FontWeight.bold);

/// Italic text style.
const _italicStyle = TextStyle(fontStyle: FontStyle.italic);

/// Strikethrough text style.
const _strikethroughStyle = TextStyle(decoration: TextDecoration.lineThrough);

/// Renders the transcript as a scrollable list.
final class MessageList extends StatelessComponent {
  /// Creates a message list.
  const MessageList({super.key, required this.messages});

  /// The messages to render, in occurrence order.
  final List<ChatMessage> messages;

  @override
  Component build(BuildContext context) {
    return ListView.separated(
      itemCount: messages.length,
      itemBuilder: (context, index) => Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (messages[index].kind != ChatMessageKind.user) Text("• "),
          Expanded(child: _MessageRow(message: messages[index])),
        ],
      ),
      separatorBuilder: (_, _) => SizedBox(height: 1),
    );
  }
}

/// One rendered transcript line.
final class _MessageRow extends StatelessComponent {
  const _MessageRow({required this.message});

  final ChatMessage message;

  @override
  Component build(BuildContext context) {
    final theme = TuiTheme.of(context);
    return switch (message.kind) {
      ChatMessageKind.user => PromptLine(child: Text(message.text)),
      ChatMessageKind.assistant => MarkdownText(
        message.text,
        styleSheet: _markdownStyle(theme),
      ),
      ChatMessageKind.reasoning => Text(
        message.text,
        style: TextStyle(color: theme.outline),
      ),
      ChatMessageKind.tool => Text(
        '${message.toolName ?? 'tool'}: ${message.text}',
        style: TextStyle(color: message.isError ? theme.error : theme.primary),
      ),
      ChatMessageKind.error => Text(
        message.text,
        style: TextStyle(color: theme.error),
      ),
      ChatMessageKind.system => Text(
        message.text,
        style: TextStyle(color: theme.warning),
      ),
    };
  }
}

/// Builds the markdown style sheet for assistant messages from [theme].
MarkdownStyleSheet _markdownStyle(TuiThemeData theme) => MarkdownStyleSheet(
  h1Style: TextStyle(fontWeight: FontWeight.bold, color: theme.primary),
  h2Style: TextStyle(fontWeight: FontWeight.bold, color: theme.success),
  h3Style: TextStyle(fontWeight: FontWeight.bold, color: theme.warning),
  h4Style: _boldStyle,
  h5Style: _boldStyle,
  h6Style: _boldStyle,
  boldStyle: _boldStyle,
  italicStyle: _italicStyle,
  strikethroughStyle: _strikethroughStyle,
  codeStyle: TextStyle(color: theme.primary),
  codeBlockStyle: TextStyle(color: theme.primary),
  blockquoteStyle: TextStyle(color: theme.outline),
  linkStyle: TextStyle(
    color: theme.primary,
    decoration: TextDecoration.underline,
  ),
);

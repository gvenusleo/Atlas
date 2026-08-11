import 'package:nocterm/nocterm.dart';
// UnicodeWidth is not part of the public API; the import below reaches into
// the package's internals, which is stable within the pinned nocterm 0.8.x.
// ignore: implementation_imports
import 'package:nocterm/src/utils/unicode_width.dart';

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
    // The list is reversed so that index 0 (the newest message) renders at
    // the bottom of the viewport: as messages arrive the transcript stays
    // pinned to the latest content without any scrolling logic.
    return ListView.separated(
      reverse: true,
      itemCount: messages.length,
      itemBuilder: (context, index) {
        final message = messages[messages.length - 1 - index];
        return Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (message.kind != ChatMessageKind.user) Text('• '),
            Expanded(child: _MessageRow(message: message)),
          ],
        );
      },
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
      ChatMessageKind.reasoning => _ReasoningLine(
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

/// Renders reasoning as a single line that always shows the newest tail of
/// the message: long content is clipped at the head instead of wrapping.
final class _ReasoningLine extends StatelessComponent {
  const _ReasoningLine(this.text, {required this.style});

  final String text;
  final TextStyle? style;

  @override
  Component build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) => Text(
        tailWindow(text.replaceAll("\n", ""), constraints.maxWidth.floor()),
        maxLines: 1,
        overflow: TextOverflow.clip,
        style: style,
      ),
    );
  }
}

/// Returns the trailing part of [text] that fits within [maxWidth] columns.
///
/// Used to show the newest reasoning content in a single line: characters are
/// collected from the end until the terminal width is exhausted, so the head
/// is dropped and the tail stays visible. When the head is dropped, `...`
/// marks the elision and reserves its three columns.
String tailWindow(String text, int maxWidth) {
  if (text.isEmpty || maxWidth <= 0) {
    return '';
  }
  final runes = text.runes.toList();
  final picked = <int>[];
  var width = 0;
  for (var i = runes.length - 1; i >= 0; i--) {
    final runeWidth = UnicodeWidth.graphemeWidth(String.fromCharCode(runes[i]));
    if (width + runeWidth > maxWidth) {
      break;
    }
    picked.add(runes[i]);
    width += runeWidth;
  }
  if (picked.length == runes.length) {
    return text;
  }
  // The head was elided: reserve three columns for the marker and collect
  // the tail again within the remaining space.
  const elided = '...';
  final contentMax = maxWidth > elided.length ? maxWidth - elided.length : 0;
  picked.clear();
  width = 0;
  for (var i = runes.length - 1; i >= 0; i--) {
    final runeWidth = UnicodeWidth.graphemeWidth(String.fromCharCode(runes[i]));
    if (width + runeWidth > contentMax) {
      break;
    }
    picked.add(runes[i]);
    width += runeWidth;
  }
  return '$elided${String.fromCharCodes(picked.reversed)}';
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

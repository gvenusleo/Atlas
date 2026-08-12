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
      ChatMessageKind.tool => switch (message.toolName) {
        'shell' || 'read' || 'edit' || 'write' => _ToolLine(message: message),
        'plan' => _PlanLine(message: message),
        _ => Text(
          '${message.toolName ?? 'tool'}: ${message.text}',
          style: TextStyle(
            color: message.isError ? theme.error : theme.primary,
          ),
        ),
      },
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
  final picked = _collectWindow(runes, maxWidth, fromEnd: true);
  if (picked.length == runes.length) {
    return text;
  }
  // The head was elided: reserve three columns for the marker and collect
  // the tail again within the remaining space.
  const elided = '...';
  final contentMax = maxWidth > elided.length ? maxWidth - elided.length : 0;
  final kept = _collectWindow(runes, contentMax, fromEnd: true);
  return '$elided${String.fromCharCodes(kept.reversed)}';
}

/// Renders a known tool call as a two-line heading plus a bounded result
/// window.
///
/// The heading carries the call metadata (path, edited blocks, or written
/// lines); the result is shown as at most [maxResultLines] lines with the
/// head elided when it is longer.
final class _ToolLine extends StatelessComponent {
  /// Creates a tool line.
  const _ToolLine({required this.message});

  /// The maximum number of result lines rendered under the heading.
  static const int maxResultLines = 5;

  final ChatMessage message;

  @override
  Component build(BuildContext context) {
    final theme = TuiTheme.of(context);
    final headingStyle = TextStyle(
      color: message.isError ? theme.error : theme.primary,
    );
    final outlineStyle = TextStyle(
      color: message.isError ? theme.error : theme.outline,
    );
    final innerStyle = TextStyle(
      color: message.isError ? theme.error : theme.warning,
    );
    return LayoutBuilder(
      builder: (context, constraints) {
        final heading = _heading(
          constraints.maxWidth.floor(),
          headingStyle,
          outlineStyle,
          innerStyle,
        );
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (heading != null)
              RichText(text: heading, maxLines: 2, overflow: TextOverflow.clip),
            // Read results are file contents, which stay out of the
            // transcript; the heading lines carry the range instead.
            if (message.toolName != 'read')
              Text(
                tailLines(message.text, maxResultLines),
                maxLines: maxResultLines,
                overflow: TextOverflow.clip,
                style: outlineStyle,
              ),
          ],
        );
      },
    );
  }

  /// The heading for a known tool call, or null when the tool has no
  /// structured heading. The tool name uses [labelStyle]; the parentheses
  /// and second line use [outlineStyle]; the argument inside the
  /// parentheses uses [innerStyle].
  TextSpan? _heading(
    int width,
    TextStyle labelStyle,
    TextStyle outlineStyle,
    TextStyle innerStyle,
  ) {
    final args = message.arguments ?? const <String, Object?>{};
    switch (message.toolName) {
      case 'shell':
        final command = '${args['command'] ?? '?'}';
        // The name and parentheses reserve their columns so the command,
        // including its trailing ellipsis, always fits on one row.
        return TextSpan(
          children: [
            TextSpan(text: 'Shell', style: labelStyle),
            TextSpan(text: '(', style: outlineStyle),
            TextSpan(
              text: headWindow(command, width - 'Shell('.length - 1),
              style: innerStyle,
            ),
            TextSpan(text: ')', style: outlineStyle),
          ],
        );
      case 'read':
        final path = '${args['path'] ?? '?'}';
        final offset = args['offset'] ?? 1;
        final limit = args['limit'] ?? 2000;
        return _labeledHeading(
          'Read',
          path,
          '\noffset $offset, limit $limit',
          labelStyle: labelStyle,
          outlineStyle: outlineStyle,
          innerStyle: innerStyle,
        );
      case 'edit':
        final path = '${args['path'] ?? '?'}';
        final edits = args['edits'];
        final blocks = edits is List ? edits.length : 0;
        return _labeledHeading(
          'Edit',
          path,
          '\n$blocks text blocks',
          labelStyle: labelStyle,
          outlineStyle: outlineStyle,
          innerStyle: innerStyle,
        );
      case 'write':
        final path = '${args['path'] ?? '?'}';
        final content = args['content'];
        final lines = content is String && content.isNotEmpty
            ? '\n'.allMatches(content).length + 1
            : 0;
        return _labeledHeading(
          'Write',
          path,
          '\n$lines lines',
          labelStyle: labelStyle,
          outlineStyle: outlineStyle,
          innerStyle: innerStyle,
        );
      default:
        return null;
    }
  }

  /// Builds a `Label(value)` heading with an outline detail line, used by
  /// the file tools whose heading carries a path and a summary.
  TextSpan _labeledHeading(
    String label,
    String value,
    String detail, {
    required TextStyle labelStyle,
    required TextStyle outlineStyle,
    required TextStyle innerStyle,
  }) => TextSpan(
    children: [
      TextSpan(text: label, style: labelStyle),
      TextSpan(text: '(', style: outlineStyle),
      TextSpan(text: value, style: innerStyle),
      TextSpan(text: ')', style: outlineStyle),
      TextSpan(text: detail, style: outlineStyle),
    ],
  );
}

/// Renders a plan tool call as a title plus one line per step.
///
/// Steps carry a status symbol: `□` pending, `✔` completed with
/// strikethrough, and the current step bold in the primary color, mirroring
/// the Go reference plan block.
final class _PlanLine extends StatelessComponent {
  /// Creates a plan line.
  const _PlanLine({required this.message});

  final ChatMessage message;

  @override
  Component build(BuildContext context) {
    final theme = TuiTheme.of(context);
    final rawPlan = message.arguments?['plan'];
    final entries = rawPlan is List
        ? rawPlan.whereType<Map<dynamic, dynamic>>().toList()
        : const <Map<dynamic, dynamic>>[];
    final allCompleted =
        entries.isNotEmpty &&
        entries.every((entry) => entry['status'] == 'completed');
    final titleStyle = TextStyle(
      color: message.isError
          ? theme.error
          : allCompleted
          ? theme.success
          : theme.primary,
      fontWeight: FontWeight.bold,
    );
    return LayoutBuilder(
      builder: (context, constraints) {
        final width = constraints.maxWidth.floor();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(message.isError ? 'Plan failed' : 'Plan', style: titleStyle),
            if (entries.isEmpty)
              Text(
                '(no steps provided)',
                style: TextStyle(color: theme.outline),
              )
            else
              for (final entry in entries) _step(entry, theme, width),
          ],
        );
      },
    );
  }

  /// Renders one plan step on a single line with its status symbol.
  ///
  /// The symbol reserves two columns; the step text is head-truncated so a
  /// long step never wraps.
  Component _step(Map<Object?, Object?> entry, TuiThemeData theme, int width) {
    final status = entry['status'];
    final outline = TextStyle(color: theme.outline);
    final step = headWindow('${entry['step'] ?? ''}', width - 2);
    switch (status) {
      case 'completed':
        return RichText(
          text: TextSpan(
            children: [
              TextSpan(
                text: '✔ ',
                style: TextStyle(color: theme.success),
              ),
              TextSpan(
                text: step,
                style: outline.copyWith(decoration: TextDecoration.lineThrough),
              ),
            ],
          ),
        );
      case 'in_progress':
        final active = TextStyle(
          color: theme.primary,
          fontWeight: FontWeight.bold,
        );
        return RichText(
          text: TextSpan(
            children: [
              TextSpan(text: '□ ', style: active),
              TextSpan(text: step, style: active),
            ],
          ),
        );
      default:
        return RichText(
          text: TextSpan(
            children: [
              TextSpan(text: '□ ', style: outline),
              TextSpan(text: step, style: outline),
            ],
          ),
        );
    }
  }
}

/// Returns the leading part of [text] that fits within [maxWidth] columns.
///
/// Characters are collected from the start until the terminal width is
/// exhausted, so the tail is dropped and the head stays visible. When the
/// tail is dropped, `...` marks the elision and reserves its three columns.
String headWindow(String text, int maxWidth) {
  if (text.isEmpty || maxWidth <= 0) {
    return '';
  }
  final runes = text.runes.toList();
  final picked = _collectWindow(runes, maxWidth, fromEnd: false);
  if (picked.length == runes.length) {
    return text;
  }
  // The tail was elided: reserve three columns for the marker and collect
  // the head again within the remaining space.
  const elided = '...';
  final contentMax = maxWidth > elided.length ? maxWidth - elided.length : 0;
  final kept = _collectWindow(runes, contentMax, fromEnd: false);
  return '${String.fromCharCodes(kept)}$elided';
}

/// Collects whole code points from [runes] while they fit within
/// [maxWidth] columns, walking from the head or from the tail (in reverse
/// order).
List<int> _collectWindow(
  List<int> runes,
  int maxWidth, {
  required bool fromEnd,
}) {
  final source = fromEnd ? runes.reversed : runes;
  final picked = <int>[];
  var width = 0;
  for (final rune in source) {
    final runeWidth = UnicodeWidth.graphemeWidth(String.fromCharCode(rune));
    if (width + runeWidth > maxWidth) {
      break;
    }
    picked.add(rune);
    width += runeWidth;
  }
  return picked;
}

/// Keeps the trailing [maxLines] lines of [text], replacing the elided head
/// with an ellipsis marker line so long results stay readable. A trailing
/// newline is not counted as an extra line.
String tailLines(String text, int maxLines) {
  var lines = text.split('\n');
  if (lines.isNotEmpty && lines.last.isEmpty) {
    lines = lines.sublist(0, lines.length - 1);
  }
  if (lines.length <= maxLines) {
    return lines.join('\n');
  }
  final kept = lines.sublist(lines.length - maxLines + 1);
  return '...\n${kept.join('\n')}';
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

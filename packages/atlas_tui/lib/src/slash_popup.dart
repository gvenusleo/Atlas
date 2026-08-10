import 'package:nocterm/nocterm.dart';
// UnicodeWidth is not part of the public API; the import below reaches into
// the package's internals, which is stable within the pinned nocterm 0.8.x.
// ignore: implementation_imports
import 'package:nocterm/src/utils/unicode_width.dart';

import 'prompt_line.dart';
import 'slash_commands.dart';

/// Maximum number of command rows shown at once.
const maxSlashPopupRows = 5;

/// Renders the slash command completion popup above the input bar.
final class SlashPopup extends StatelessComponent {
  /// Creates a slash popup.
  const SlashPopup({
    super.key,
    required this.matches,
    required this.selected,
    this.showSlash = true,
    this.title,
  });

  /// The ranked command matches.
  final List<SlashCommand> matches;

  /// The index of the highlighted command.
  final int selected;

  /// Whether names are rendered with a leading `/`; hidden for the model
  /// picker, where the names are display labels.
  final bool showSlash;

  /// An optional heading line above the rows (e.g. the reasoning stage).
  final String? title;

  @override
  Component build(BuildContext context) {
    final theme = TuiTheme.of(context);
    final start = _windowStart(matches.length, selected, maxSlashPopupRows);
    final end = _min(start + maxSlashPopupRows, matches.length);

    // Column width over the visible window so names left-align and
    // descriptions start at the same column, like the Go reference.
    var nameColumnWidth = 0;
    for (var i = start; i < end; i++) {
      final width = UnicodeWidth.stringWidth(_displayName(matches[i]));
      if (width > nameColumnWidth) {
        nameColumnWidth = width;
      }
    }

    final rows = <Component>[];
    for (var i = start; i < end; i++) {
      final command = matches[i];
      final highlighted = i == selected;
      final name = _displayName(command);
      final padding = _spaces(nameColumnWidth - UnicodeWidth.stringWidth(name));
      final nameStyle = TextStyle(
        color: highlighted ? theme.primary : theme.onBackground,
        fontWeight: highlighted ? FontWeight.bold : FontWeight.normal,
      );
      final spans = <TextSpan>[
        TextSpan(text: highlighted ? '› ' : '  ', style: nameStyle),
        TextSpan(text: name, style: nameStyle),
      ];
      if (command.description.isNotEmpty) {
        spans
          ..add(TextSpan(text: '$padding  ', style: nameStyle))
          ..add(
            TextSpan(
              text: command.description,
              style: TextStyle(color: theme.outline),
            ),
          );
      }
      rows.add(RichText(text: TextSpan(children: spans)));
    }
    return Container(
      width: double.maxFinite,
      color: withGrayOverlay(TuiTheme.of(context).background),
      padding: const EdgeInsets.only(top: 1),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (title != null)
            Text(title!, style: TextStyle(color: theme.outline)),
          ...rows,
        ],
      ),
    );
  }

  String _displayName(SlashCommand command) =>
      showSlash ? '/${command.name}' : command.name;
}

/// Start row of the visible window so the selection stays on screen.
int _windowStart(int count, int selected, int maxRows) {
  if (count <= maxRows) {
    return 0;
  }
  final top = selected - (maxRows - 1) ~/ 2;
  return top.clamp(0, count - maxRows);
}

int _min(int a, int b) => a < b ? a : b;

String _spaces(int count) => List.filled(count, ' ').join();

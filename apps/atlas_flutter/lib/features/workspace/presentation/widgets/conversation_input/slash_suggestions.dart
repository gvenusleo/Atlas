import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';
import '../../workspace_metrics.dart';
import 'floating_menu_card.dart';

/// Maximum slash suggestion rows shown before the popup scrolls.
const maxSlashPopupRows = 5;

/// Slash commands built into the composer itself.
const builtInSlashCommands = <(String, String)>[
  ('compact', 'Compact the conversation'),
];

/// The suggestion popup shown while a slash token is being typed.
class SlashSuggestions extends StatelessWidget {
  /// Creates a slash suggestion popup.
  const SlashSuggestions({
    super.key,
    required this.suggestions,
    required this.selected,
    required this.onHighlighted,
    required this.onSelected,
  });

  static const _rowHeight = 30.0;

  final List<(String, String)> suggestions;
  final int selected;
  final ValueChanged<int> onHighlighted;
  final ValueChanged<(String, String)> onSelected;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      child: FloatingMenuCard(
        items: suggestions,
        selectedIndex: 0,
        highlighted: selected,
        onHighlighted: onHighlighted,
        onSelected: onSelected,
        rowHeight: _rowHeight,
        cardPadding: 8,
        maxVisibleRows: maxSlashPopupRows,
        itemBuilder: (context, suggestion, index) => Row(
          children: [
            Text(
              '/${suggestion.$1}',
              style: TextStyle(
                color: colors.textPrimary,
                fontFamily: WorkspaceMetrics.monospaceFontFamily,
                fontSize: 12.5,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                suggestion.$2,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(color: colors.textSecondary, fontSize: 12),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// The slash token under [offset], or null when the cursor is not inside a
/// `/name` token.
///
/// Mirrors the TUI completer: a token whose surroundings are not all
/// whitespace only completes skills, so built-ins stay whole-line commands.
({int start, int end, String query, bool skillsOnly})? slashTokenAt(
  String text,
  int offset,
) {
  if (text.isEmpty) {
    return null;
  }
  final column = offset.clamp(0, text.length);
  if (column > 0 && _isSeparator(text[column - 1])) {
    return null;
  }
  var start = column;
  while (start > 0 && !_isSeparator(text[start - 1])) {
    start--;
  }
  var end = column;
  while (end < text.length && !_isSeparator(text[end])) {
    end++;
  }
  if (start >= end || text[start] != '/') {
    return null;
  }
  final query = text.substring(start + 1, end);
  if (query.isNotEmpty && !_isValidCommandName(query)) {
    return null;
  }
  final skillsOnly = '${text.substring(0, start)}${text.substring(end)}'
      .trim()
      .isNotEmpty;
  return (start: start, end: end, query: query, skillsOnly: skillsOnly);
}

/// Ranks slash commands: exact name first, then prefix, then substring.
List<(String, String, bool)> rankSlashCommands(
  String query,
  List<(String, String, bool)> commands,
) {
  if (query.isEmpty) {
    return List.of(commands);
  }
  final exact = <(String, String, bool)>[];
  final prefix = <(String, String, bool)>[];
  final contains = <(String, String, bool)>[];
  final lower = query.toLowerCase();
  for (final command in commands) {
    final name = command.$1.toLowerCase();
    if (name == lower) {
      exact.add(command);
    } else if (name.startsWith(lower)) {
      prefix.add(command);
    } else if (name.contains(lower)) {
      contains.add(command);
    }
  }
  return [...exact, ...prefix, ...contains];
}

/// Whether [char] is a whitespace separator delimiting slash tokens.
bool _isSeparator(String char) =>
    char == ' ' || char == '\t' || char == '\n' || char == '\r';

/// Whether [name] is a valid slash token, matching the TUI character set.
bool _isValidCommandName(String name) {
  if (name.isEmpty) {
    return false;
  }
  for (final code in name.codeUnits) {
    final isLetter =
        (code >= 0x41 && code <= 0x5A) || (code >= 0x61 && code <= 0x7A);
    final isDigit = code >= 0x30 && code <= 0x39;
    if (!isLetter && !isDigit && code != 0x5F && code != 0x2D && code != 0x2E) {
      return false;
    }
  }
  return true;
}

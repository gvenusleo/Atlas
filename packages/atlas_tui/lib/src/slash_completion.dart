import 'slash_commands.dart';

/// Describes the slash token at the cursor in an editing buffer.
final class SlashToken {
  /// Creates a slash token descriptor.
  const SlashToken({
    required this.start,
    required this.end,
    required this.query,
  });

  /// Start offset of the token (including the `/`).
  final int start;

  /// End offset of the token (exclusive).
  final int end;

  /// The token text without the leading `/`.
  final String query;

  @override
  bool operator ==(Object other) =>
      other is SlashToken &&
      other.start == start &&
      other.end == end &&
      other.query == query;

  @override
  int get hashCode => Object.hash(start, end, query);

  @override
  String toString() => 'SlashToken($start..$end, "/$query")';
}

/// Completion state for slash commands in the message input.
///
/// Owns command matching, selection, and token replacement while the cursor
/// sits on a `/`-prefixed token. It is pure Dart so terminal behavior can be
/// tested without a rendering surface.
final class SlashCompleter {
  /// Creates a completer over [commands] (defaults to the built-in catalog).
  SlashCompleter({List<SlashCommand> commands = slashCommands})
    : _commands = List.unmodifiable(commands);

  final List<SlashCommand> _commands;
  List<SlashCommand> _matches = const [];
  int _selected = 0;
  SlashToken? _target;
  String _dismissedValue = '';
  SlashToken? _dismissedTarget;

  /// The commands available for completion.
  List<SlashCommand> get commands => _commands;

  /// The commands matching the current query, in ranked order.
  List<SlashCommand> get matches => _matches;

  /// The index of the selected match.
  int get selected => _selected;

  /// The active slash token, or `null` when no popup is shown.
  SlashToken? get target => _target;

  /// Whether a completion popup should be visible.
  bool get active => _matches.isNotEmpty;

  /// The selected match, or `null` when the popup is inactive.
  SlashCommand? get selectedCommand =>
      active && _selected >= 0 && _selected < _matches.length
      ? _matches[_selected]
      : null;

  /// Recomputes matches for the slash token at [offset] in [text].
  ///
  /// Called after every edit. A dismissed popup stays closed for the same
  /// draft and token, and reopens as soon as either changes.
  void sync(String text, int offset) {
    final token = slashTokenAt(text, offset);
    if (_dismissedValue.isNotEmpty &&
        (text != _dismissedValue || token != _dismissedTarget)) {
      _dismissedValue = '';
      _dismissedTarget = null;
    }
    if (token == null ||
        (text == _dismissedValue && token == _dismissedTarget)) {
      _matches = const [];
      _selected = 0;
      _target = null;
      return;
    }
    final targetChanged = token != _target;
    _target = token;
    final query = token.query.toLowerCase();
    _matches = _rank(query, _commands);
    if (targetChanged) {
      _selected = 0;
    } else {
      _selected = _selected.clamp(0, _matches.length - 1);
    }
  }

  /// Moves the selection by [delta] within the current matches.
  void move(int delta) {
    if (!active) {
      return;
    }
    _selected = (_selected + delta).clamp(0, _matches.length - 1);
  }

  /// Closes the popup until the draft or token changes.
  void dismiss(String text) {
    _dismissedValue = text;
    _dismissedTarget = _target;
    _matches = const [];
    _selected = 0;
    _target = null;
  }

  /// Shows every command without requiring a slash token.
  ///
  /// Used by the model picker, where the input is read-only and the list is
  /// the whole catalog.
  void showAll() {
    _dismissedValue = '';
    _dismissedTarget = null;
    _matches = _rank('', _commands);
    _selected = 0;
    _target = null;
  }

  /// Replaces the active slash token with `/name`, preserving the rest of the
  /// draft. Returns the new buffer text and cursor offset.
  ///
  /// Functions like the Go reference: a trailing space is inserted after the
  /// replacement so the user can keep typing arguments.
  ({String text, int offset}) applyToken(String text, String name) {
    final token = _target;
    if (token == null) {
      return (text: text, offset: text.length);
    }
    final replacement = '/$name';
    var newText =
        text.substring(0, token.start) +
        replacement +
        text.substring(token.end);
    var newOffset = token.start + replacement.length;
    if (newOffset >= newText.length) {
      newText = '$newText ';
      newOffset = newText.length;
    } else if (newText[newOffset] == ' ') {
      newOffset += 1;
    } else {
      newText =
          '${newText.substring(0, newOffset)} ${newText.substring(newOffset)}';
      newOffset += 1;
    }
    return (text: newText, offset: newOffset);
  }
}

/// Locates the whitespace-delimited slash token under [offset] in [text].
///
/// Follows the Go reference semantics: the character before the cursor must
/// not be whitespace, the token must start with `/`, and a non-empty query
/// must consist of valid command-name characters.
SlashToken? slashTokenAt(String text, int offset) {
  if (text.isEmpty) {
    return null;
  }
  final column = offset.clamp(0, text.length);
  if (column > 0 && _isWhitespace(text[column - 1])) {
    return null;
  }
  var start = column;
  while (start > 0 && !_isWhitespace(text[start - 1])) {
    start--;
  }
  var end = column;
  while (end < text.length && !_isWhitespace(text[end])) {
    end++;
  }
  if (start >= end || text[start] != '/') {
    return null;
  }
  final query = text.substring(start + 1, end);
  if (query.isNotEmpty && !validSlashCommandName(query)) {
    return null;
  }
  return SlashToken(start: start, end: end, query: query);
}

/// Whether [char] is a whitespace separator. A single code unit is enough for
/// the ASCII separators (space, tab, newline) that delimit slash tokens.
bool _isWhitespace(String char) =>
    char == ' ' || char == '\t' || char == '\n' || char == '\r';

/// Ranks matches: exact name first, then prefix, then substring.
List<SlashCommand> _rank(String query, List<SlashCommand> commands) {
  if (query.isEmpty) {
    return List.of(commands);
  }
  final exact = <SlashCommand>[];
  final prefix = <SlashCommand>[];
  final contains = <SlashCommand>[];
  for (final command in commands) {
    final name = command.name.toLowerCase();
    if (name == query) {
      exact.add(command);
    } else if (name.startsWith(query)) {
      prefix.add(command);
    } else if (name.contains(query)) {
      contains.add(command);
    }
  }
  return [...exact, ...prefix, ...contains];
}

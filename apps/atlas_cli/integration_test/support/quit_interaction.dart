/// Drives a real TUI input field using rendered cursor acknowledgements.
///
/// Nocterm batches text and Enter received together into a paste. Send the
/// command with trailing whitespace (which dismisses slash completion), wait
/// until its caret is visible after that whitespace, then send Enter alone.
class QuitInteraction({final bool fragmented = false}) {
  static const _command = '/quit ';
  var _sent = 0;
  var _submitted = false;

  /// Reports the last acknowledged stage when a PTY test times out.
  String get stage => _submitted
      ? 'waiting-for-exit'
      : _sent == 0
      ? 'waiting-for-prompt'
      : 'waiting-for-command-caret:$_sent';

  /// Returns the next input bytes only after the previous input was rendered.
  String? advance(String output) {
    if (_submitted) return null;
    final screen = _Screen(output);
    final beforeCaret = screen.beforeCaret;
    if (_sent == 0) {
      if (beforeCaret != '› ' || !screen.line.contains('Message Atlas')) {
        return null;
      }
    } else if (beforeCaret != '› ${_command.substring(0, _sent)}') {
      return null;
    }
    if (_sent == _command.length) {
      _submitted = true;
      return '\r';
    }
    final next = fragmented ? _command[_sent] : _command;
    _sent += next.length;
    return next;
  }
}

// Minimal screen reader for the PTY fixture's fixed 100x32 ASCII command field.
// Interpret cursor positioning instead of stripping escapes: Nocterm renders
// changes cell by cell, and stale popup text is not an input acknowledgement.
class _Screen(String output) {
  this {
    var offset = 0;
    while (offset < output.length) {
      if (output.codeUnitAt(offset) == 0x1b) {
        final match = _escape.matchAsPrefix(output, offset);
        if (match == null) break; // Wait for a fragmented escape sequence.
        final sequence = match.group(0)!;
        if (sequence.startsWith('\x1b[')) {
          final parameters = sequence.substring(2, sequence.length - 1);
          final end = sequence[sequence.length - 1];
          if (end == 'H' || end == 'f') {
            final parts = parameters.split(';');
            _row = ((int.tryParse(parts.first) ?? 1) - 1).clamp(0, 31);
            _column = ((parts.length > 1 ? int.tryParse(parts[1]) ?? 1 : 1) - 1)
                .clamp(0, 99);
          } else if (end == 'J' && parameters == '2') {
            for (final row in _cells) {
              row.fillRange(0, row.length, ' ');
            }
          } else if (end == 'K') {
            final start = parameters == '1' || parameters == '2' ? 0 : _column;
            final stop = parameters == '1' ? _column + 1 : 100;
            _cells[_row].fillRange(start, stop, ' ');
          }
        }
        offset = match.end;
        continue;
      }
      final char = output[offset++];
      if (char == '\r') {
        _column = 0;
      } else if (char == '\n') {
        _row = (_row + 1).clamp(0, 31);
      } else if (char.codeUnitAt(0) >= 32 && _column < 100) {
        _cells[_row][_column++] = char;
      }
    }
  }

  static final _escape = RegExp(
    r'\x1b\[[0-?]*[ -/]*[@-~]|\x1b[\]P_][\s\S]*?(?:\x07|\x1b\\)|\x1b(?![\[\]P_])[@-_]',
  );
  final _cells = List.generate(32, (_) => List.filled(100, ' '));
  var _row = 0;
  var _column = 0;
  String get beforeCaret => _cells[_row].take(_column).join();
  String get line => _cells[_row].join();
}

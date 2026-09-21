import 'dart:collection';
import 'dart:convert';

/// Retains bounded UTF-8 head/tail bytes without splitting output characters.
final class ShellOutputBuffer(
  /// Maximum encoded size of the rendered output.
  final int limit,
) {
  /// Creates a buffer including truncation text; [limit] must be at least 55.
  this {
    if (limit < _marker.length * 2 - 1) {
      throw ArgumentError.value(limit, 'limit', 'must be at least 55 bytes');
    }
  }

  final _head = <int>[];
  final _tail = ListQueue<int>();
  int _length = 0;
  static const _marker = '\n... [output truncated] ...\n';

  /// Whether output has exceeded the retained capacity.
  bool get truncated => _length > limit;

  /// Number of bytes retained internally, independent of total output size.
  int get retainedBytes => _head.length + _tail.length;

  /// Adds already decoded and filtered text to the bounded buffer.
  void add(String text) {
    final bytes = utf8.encode(text);
    _length += bytes.length;
    for (final byte in bytes) {
      if (_head.length < limit ~/ 2) {
        _head.add(byte);
      } else {
        if (_tail.length == limit - limit ~/ 2) _tail.removeFirst();
        _tail.addLast(byte);
      }
    }
  }

  /// Renders a replacement snapshot of the retained output.
  String get text {
    if (!truncated) return utf8.decode([..._head, ..._tail]);
    final tail = _tail.toList();
    var start = _marker.length;
    while (start < tail.length && tail[start] & 0xc0 == 0x80) {
      start++;
    }
    var end = _head.length;
    var last = end - 1;
    while (last >= 0 && _head[last] & 0xc0 == 0x80) {
      last--;
    }
    if (last >= 0) {
      final byte = _head[last];
      final width = byte < 0x80 ? 1 : (byte < 0xe0 ? 2 : (byte < 0xf0 ? 3 : 4));
      if (last + width > end) end = last;
    }
    return '${utf8.decode(_head.sublist(0, end))}$_marker'
        '${utf8.decode(tail.sublist(start))}';
  }
}

/// Stateful plain-text filter for terminal controls split across pipe chunks.
final class ShellTextFilter {
  var _state = _EscapeState.text;
  var _carriageReturn = false;

  /// Removes escape/control sequences and normalizes CR/CRLF to line breaks.
  String add(String text) {
    final result = StringBuffer();
    for (final rune in text.runes) {
      if (rune == 0x1b) {
        _state = _state == _EscapeState.string
            ? _EscapeState.stringEscape
            : _EscapeState.escape;
        continue;
      }
      switch (_state) {
        case _EscapeState.escape:
          _state = switch (rune) {
            0x5b => _EscapeState.csi,
            0x5d || 0x50 || 0x5e || 0x5f => _EscapeState.string,
            >= 0x20 && <= 0x2f => _EscapeState.escape,
            _ => _EscapeState.text,
          };
          continue;
        case _EscapeState.csi:
          if (rune >= 0x40 && rune <= 0x7e) _state = _EscapeState.text;
          continue;
        case _EscapeState.string:
          if (rune == 7 || rune == 0x9c) _state = _EscapeState.text;
          continue;
        case _EscapeState.stringEscape:
          _state = rune == 0x5c ? _EscapeState.text : _EscapeState.string;
          continue;
        case _EscapeState.text:
          break;
      }
      if (rune == 0x9b) {
        _state = _EscapeState.csi;
      } else if (rune == 0x9d || rune == 0x90 || rune == 0x9e || rune == 0x9f) {
        _state = _EscapeState.string;
      } else if (rune == 13) {
        result.write('\n');
      } else if (rune == 10) {
        if (!_carriageReturn) result.write('\n');
      } else if (rune == 9 ||
          (rune >= 0x20 && !(rune >= 0x7f && rune <= 0x9f))) {
        result.writeCharCode(rune);
      }
      _carriageReturn = rune == 13;
    }
    return result.toString();
  }
}

enum _EscapeState { text, escape, csi, string, stringEscape }

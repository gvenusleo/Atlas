import 'dart:convert';

/// Decodes a JSON document with Pi-style line and block comments.
///
/// Comments are replaced with whitespace so parser offsets remain useful.
/// Trailing commas are rejected, as in Pi's JSON parser.
Map<String, Object?> decodeJsonDocument(String source) {
  final text = source.startsWith('\uFEFF') ? source.substring(1) : source;
  final chars = text.split('');
  var quoted = false;
  for (var i = 0; i < chars.length; i++) {
    if (quoted) {
      if (chars[i] == r'\') {
        i++;
      } else if (chars[i] == '"') {
        quoted = false;
      }
      continue;
    }
    if (chars[i] == '"') {
      quoted = true;
      continue;
    }
    if (chars[i] != '/' || i + 1 >= chars.length) continue;
    if (chars[i + 1] == '/') {
      while (i < chars.length && chars[i] != '\n' && chars[i] != '\r') {
        chars[i++] = ' ';
      }
      i--;
    } else if (chars[i + 1] == '*') {
      final start = i;
      chars[i++] = ' ';
      chars[i++] = ' ';
      while (i + 1 < chars.length &&
          !(chars[i] == '*' && chars[i + 1] == '/')) {
        if (chars[i] != '\n' && chars[i] != '\r') chars[i] = ' ';
        i++;
      }
      if (i + 1 >= chars.length) {
        throw FormatException('Unterminated comment', text, start);
      }
      chars[i] = ' ';
      chars[++i] = ' ';
    }
  }
  final value = jsonDecode(chars.join());
  if (value is! Map<String, Object?>) {
    throw const FormatException('Expected a JSON object');
  }
  return value;
}

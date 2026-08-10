/// Shared text handling for local file tools.
library;

/// The UTF-8 byte-order mark.
const utf8BomBytes = [0xEF, 0xBB, 0xBF];

/// Returns whether [bytes] starts with a UTF-8 byte-order mark.
bool hasUtf8Bom(List<int> bytes) {
  if (bytes.length < utf8BomBytes.length) {
    return false;
  }
  for (var index = 0; index < utf8BomBytes.length; index++) {
    if (bytes[index] != utf8BomBytes[index]) {
      return false;
    }
  }
  return true;
}

/// Returns the dominant line ending of [text]: `\r\n` or `\n`.
///
/// Ties and line-free text default to `\n`.
String primaryLineEnding(String text) {
  final crlf = '\r\n'.allMatches(text).length;
  final lf = '\n'.allMatches(text).length - crlf;
  return crlf > lf ? '\r\n' : '\n';
}

/// Normalizes every line ending to `\n` (CRLF and lone CR included).
String normalizeNewlines(String text) =>
    text.replaceAll('\r\n', '\n').replaceAll('\r', '\n');

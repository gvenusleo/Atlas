/// Estimates tokens using the shared four-characters-per-token heuristic.
int estimateTokenCount(String text) {
  var cjk = 0;
  var other = 0;
  for (final rune in text.runes) {
    final isCjk =
        (rune >= 0x4e00 && rune <= 0x9fff) ||
        (rune >= 0x3400 && rune <= 0x4dbf) ||
        (rune >= 0xac00 && rune <= 0xd7af) ||
        (rune >= 0x3040 && rune <= 0x30ff);
    if (isCjk) {
      cjk++;
    } else {
      other++;
    }
  }
  return cjk + (other / 4).ceil();
}

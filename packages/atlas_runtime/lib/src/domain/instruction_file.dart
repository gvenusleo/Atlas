/// One loaded AGENTS.md instruction file.
final class const InstructionFile({
  /// The absolute file path.
  required final String path,

  /// The file content, bounded to [maxBytes].
  required final String content,
}) {
  /// Creates an instruction file.
  this;

  /// The maximum instruction content loaded into the system prompt.
  static const int maxBytes = 64 * 1024;
}

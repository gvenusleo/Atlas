/// One loaded AGENTS.md instruction file.
final class InstructionFile {
  /// Creates an instruction file.
  const InstructionFile({required this.path, required this.content});

  /// The absolute file path.
  final String path;

  /// The file content, bounded to [maxBytes].
  final String content;

  /// The maximum instruction content loaded into the system prompt.
  static const int maxBytes = 64 * 1024;
}

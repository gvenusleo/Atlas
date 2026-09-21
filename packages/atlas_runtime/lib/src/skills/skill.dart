/// A local skill loaded from a SKILL.md directory.
final class const Skill({
  /// The skill name from the frontmatter.
  required final String name,

  /// The model-visible skill summary.
  required final String description,

  /// The skill directory containing SKILL.md.
  required final String dir,

  /// The absolute SKILL.md path.
  required final String path,

  /// The full SKILL.md content, bounded to [maxSkillBytes].
  required final String content,

  /// When true, the model is not told about this skill and cannot select it.
  final bool disableModelInvocation = false,
}) {
  /// Creates a skill.
  this;

  /// The maximum SKILL.md content loaded into the model context.
  static const int maxBytes = 64 * 1024;
}

/// The model-visible summary of an available skill.
final class const SkillSummary({
  /// The skill name.
  required final String name,

  /// The SKILL.md path, so the model can read it with the read tool.
  required final String path,

  /// The skill description.
  required final String description,
}) {
  /// Creates a skill summary.
  this;
}

/// A slash command advertised by an agent for a session.
final class const AgentCommand({
  /// The command name, without the leading slash.
  required final String name,

  /// Human-readable description of what the command does.
  required final String description,

  /// Optional hint shown while the user has not typed input yet.
  final String inputHint = '',
}) {
  /// Creates an agent command.
  this;
}

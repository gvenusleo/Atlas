/// A local skill loaded from a SKILL.md directory.
final class Skill {
  /// Creates a skill.
  const Skill({
    required this.name,
    required this.description,
    required this.dir,
    required this.path,
    required this.content,
    this.disableModelInvocation = false,
  });

  /// The skill name from the frontmatter.
  final String name;

  /// The model-visible skill summary.
  final String description;

  /// The skill directory containing SKILL.md.
  final String dir;

  /// The absolute SKILL.md path.
  final String path;

  /// The full SKILL.md content, bounded to [maxSkillBytes].
  final String content;

  /// When true, the model is not told about this skill and cannot select it.
  final bool disableModelInvocation;

  /// The maximum SKILL.md content loaded into the model context.
  static const int maxBytes = 64 * 1024;
}

/// The model-visible summary of an available skill.
final class SkillSummary {
  /// Creates a skill summary.
  const SkillSummary({
    required this.name,
    required this.path,
    required this.description,
  });

  /// The skill name.
  final String name;

  /// The SKILL.md path, so the model can read it with the read tool.
  final String path;

  /// The skill description.
  final String description;
}

/// A slash command advertised by an agent for a session.
final class AgentCommand {
  /// Creates an agent command.
  const AgentCommand({
    required this.name,
    required this.description,
    this.inputHint = '',
  });

  /// The command name, without the leading slash.
  final String name;

  /// Human-readable description of what the command does.
  final String description;

  /// Optional hint shown while the user has not typed input yet.
  final String inputHint;
}

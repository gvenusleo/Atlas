import 'instruction_file.dart';
import '../skills/skill_catalog.dart';

/// The filesystem-backed context of one session, loaded once when the
/// session working directory is fixed.
final class SessionContext {
  /// Creates a session context.
  const SessionContext({
    required this.workingDirectory,
    required this.instructions,
    required this.skills,
  });

  /// The immutable session working directory.
  final String workingDirectory;

  /// The AGENTS.md instruction files scoped to [workingDirectory].
  final List<InstructionFile> instructions;

  /// The skills scoped to [workingDirectory].
  final SkillCatalog skills;
}

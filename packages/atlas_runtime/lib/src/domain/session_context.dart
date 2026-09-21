import 'instruction_file.dart';
import '../skills/skill_catalog.dart';

/// The filesystem-backed context of one session, loaded once when the
/// session working directory is fixed.
final class const SessionContext({
  /// The immutable session working directory.
  required final String workingDirectory,

  /// The AGENTS.md instruction files scoped to [workingDirectory].
  required final List<InstructionFile> instructions,

  /// The skills scoped to [workingDirectory].
  required final SkillCatalog skills,
}) {
  /// Creates a session context.
  this;
}

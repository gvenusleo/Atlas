import 'package:atlas_runtime/atlas_runtime.dart';

import 'instruction_files.dart';
import 'skill_loader.dart';

/// Loads the complete filesystem context for a session working directory.
///
/// Merges the AGENTS.md instruction files and the skill catalog; both are
/// scoped to [workingDirectory] and loaded once per session.
SessionContext buildSessionContext(String workingDirectory) => SessionContext(
  workingDirectory: workingDirectory,
  instructions: loadInstructionFiles(workingDirectory: workingDirectory),
  skills: loadSkillCatalog(workingDirectory: workingDirectory),
);

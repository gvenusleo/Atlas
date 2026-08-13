import 'skill.dart';

/// The skills available to one working directory.
///
/// Implementations load and parse SKILL.md files; the runtime consumes the
/// catalog to answer model-visible summaries and user-selected skills.
abstract interface class SkillCatalog {
  /// Model-visible summaries in stable name order, skipping disabled skills.
  List<SkillSummary> get summaries;

  /// Returns the loadable skill by name, or null when unknown or disabled.
  Skill? lookup(String name);
}

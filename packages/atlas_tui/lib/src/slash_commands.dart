import 'package:atlas_runtime/atlas_runtime.dart';

/// A built-in slash command recognized by the input bar.
final class SlashCommand {
  /// Creates a slash command.
  const SlashCommand({required this.name, this.description = ''});

  /// The command name without the leading `/`.
  final String name;

  /// One-line description shown in the completion popup.
  final String description;
}

/// The built-in command catalog.
const slashCommands = [
  SlashCommand(name: 'model', description: 'Choose a model'),
  SlashCommand(name: 'new', description: 'Start a new session'),
  SlashCommand(name: 'quit', description: 'Quit Atlas'),
];

/// Parses a whole-line slash command from [text].
///
/// Returns the command when [text] is exactly `/name` (allowing surrounding
/// whitespace), and `null` otherwise so the text can be submitted as a normal
/// message.
SlashCommand? parseSlashCommand(String text) {
  final trimmed = text.trim();
  if (!trimmed.startsWith('/')) {
    return null;
  }
  final name = trimmed.substring(1);
  for (final command in slashCommands) {
    if (command.name == name) {
      return command;
    }
  }
  return null;
}

/// Whether [name] is safe to use as a slash command name.
bool validSlashCommandName(String name) {
  if (name.isEmpty) {
    return false;
  }
  for (final code in name.codeUnits) {
    final isLetter =
        (code >= 0x41 && code <= 0x5A) || (code >= 0x61 && code <= 0x7A);
    final isDigit = code >= 0x30 && code <= 0x39;
    if (!isLetter && !isDigit && code != 0x5F && code != 0x2D && code != 0x2E) {
      return false;
    }
  }
  return true;
}

/// Resolves a whole-line or leading `/name` skill command from [text].
///
/// Returns the matching skill when [text] starts with `/name` and [catalog]
/// knows the skill; `null` otherwise so the text can be submitted as a
/// normal message. Built-in commands take precedence via [parseSlashCommand].
Skill? parseSkillCommand(String text, SkillCatalog? catalog) {
  if (catalog == null) {
    return null;
  }
  final trimmed = text.trim();
  if (!trimmed.startsWith('/')) {
    return null;
  }
  final end = trimmed.indexOf(RegExp(r'[\s]'), 1);
  final name = end < 0 ? trimmed.substring(1) : trimmed.substring(1, end);
  if (!validSlashCommandName(name)) {
    return null;
  }
  return catalog.lookup(name);
}

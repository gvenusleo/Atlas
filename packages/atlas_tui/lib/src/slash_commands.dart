/// A slash command recognized by the input bar.
final class SlashCommand {
  /// Creates a slash command.
  const SlashCommand({
    required this.name,
    this.description = '',
    this.isSkill = false,
  });

  /// The command name without the leading `/`.
  final String name;

  /// One-line description shown in the completion popup.
  final String description;

  /// Whether this command is a skill trigger rather than a built-in command.
  final bool isSkill;
}

/// The name of the `/model` command.
const modelCommandName = 'model';

/// The name of the `/new` command.
const newCommandName = 'new';

/// The name of the `/resume` command.
const resumeCommandName = 'resume';

/// The name of the `/compact` command.
const compactCommandName = 'compact';

/// The name of the `/quit` command.
const quitCommandName = 'quit';

/// The built-in command catalog.
const slashCommands = [
  SlashCommand(
    name: compactCommandName,
    description: 'Compact the conversation',
  ),
  SlashCommand(name: modelCommandName, description: 'Choose a model'),
  SlashCommand(name: newCommandName, description: 'Start a new session'),
  SlashCommand(name: quitCommandName, description: 'Quit Atlas'),
  SlashCommand(
    name: resumeCommandName,
    description: 'Resume a previous session',
  ),
];

/// The built-in command names, used to keep skills from shadowing them.
const reservedCommandNames = {
  modelCommandName,
  newCommandName,
  resumeCommandName,
  compactCommandName,
  quitCommandName,
};

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

/// Parses a `/compact [instruction]` command from [text].
///
/// Returns the trailing instruction text (possibly empty) when [text] is a
/// compact command, or null when it is a regular prompt.
String? compactCommandInstruction(String text) {
  final trimmed = text.trim();
  if (trimmed == '/$compactCommandName') {
    return '';
  }
  if (RegExp('^/$compactCommandName\\s').hasMatch(trimmed)) {
    return trimmed.substring(compactCommandName.length + 1).trim();
  }
  return null;
}

/// Parses a `/resume [sessionId]` command from [text].
///
/// Returns the trailing session id (possibly empty) when [text] is a resume
/// command, or null when it is a regular prompt.
String? resumeCommandSessionID(String text) {
  final trimmed = text.trim();
  if (trimmed == '/$resumeCommandName') {
    return '';
  }
  if (RegExp('^/$resumeCommandName\\s').hasMatch(trimmed)) {
    return trimmed.substring(resumeCommandName.length + 1).trim();
  }
  return null;
}

/// The valid command name of [field] when it is a `/name` token, else null.
String? slashCommandName(String field) {
  if (field.length < 2 || !field.startsWith('/')) {
    return null;
  }
  final name = field.substring(1);
  if (!validSlashCommandName(name)) {
    return null;
  }
  return name;
}

/// Scans [text] for whitespace-separated `/name` tokens naming skills.
///
/// Returns the deduplicated names in order of first appearance, excluding
/// the built-in command names. Unknown names are safe to forward: the runtime
/// ignores selected skills it cannot resolve.
List<String> selectedSkillNames(String text) {
  final result = <String>[];
  final seen = <String>{};
  for (final field in text.split(RegExp(r'\s+'))) {
    final name = slashCommandName(field);
    if (name == null ||
        reservedCommandNames.contains(name) ||
        !seen.add(name)) {
      continue;
    }
    result.add(name);
  }
  return result;
}

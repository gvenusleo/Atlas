import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

/// The maximum skill description size accepted from the frontmatter.
const maxSkillDescriptionBytes = 2 * 1024;

/// The maximum total size of model-visible skill summaries.
const maxSkillSummaryBytes = 32 * 1024;

/// A [SkillCatalog] loaded from the user-level and project-level skill roots.
final class FileSkillCatalog implements SkillCatalog {
  /// Creates a catalog from the skills found under [roots].
  FileSkillCatalog(Iterable<Directory> roots) : _skills = _loadRoots(roots);

  final Map<String, Skill> _skills;

  @override
  List<SkillSummary> get summaries {
    final names = _skills.keys.toList()..sort();
    return [
      for (final name in names)
        if (!_skills[name]!.disableModelInvocation)
          SkillSummary(
            name: name,
            path: _skills[name]!.path,
            description: _skills[name]!.description,
          ),
    ];
  }

  @override
  Skill? lookup(String name) {
    final skill = _skills[name];
    if (skill == null || skill.disableModelInvocation) {
      return null;
    }
    return skill;
  }
}

/// Loads skills from `~/.atlas/skills`, `~/.agents/skills`, and the
/// `.atlas/skills` and `.agents/skills` directories under [workingDirectory].
///
/// Later roots override earlier ones for duplicate skill names. Missing root
/// directories, directories without SKILL.md, and malformed or oversized
/// SKILL.md files are skipped; the remaining skills stay usable.
FileSkillCatalog loadSkillCatalog({
  String? workingDirectory,
  String? homeDirectory,
}) {
  final home = homeDirectory ?? Platform.environment['HOME'];
  final cwd = workingDirectory ?? Directory.current.path;
  final roots = <Directory>[
    if (home != null && home.isNotEmpty) Directory('$home/.atlas/skills'),
    if (home != null && home.isNotEmpty) Directory('$home/.agents/skills'),
    Directory('$cwd/.atlas/skills'),
    Directory('$cwd/.agents/skills'),
  ];
  return FileSkillCatalog(roots);
}

Map<String, Skill> _loadRoots(Iterable<Directory> roots) {
  final byName = <String, Skill>{};
  for (final root in roots) {
    for (final skill in _loadRoot(root)) {
      byName[skill.name] = skill;
    }
  }
  _validateSummaries(byName);
  return byName;
}

List<Skill> _loadRoot(Directory root) {
  if (!root.existsSync()) {
    return const [];
  }
  final result = <Skill>[];
  for (final entry in root.listSync(followLinks: false)) {
    if (entry is! Directory) {
      continue;
    }
    final skillFile = File('${entry.path}/SKILL.md');
    if (!skillFile.existsSync()) {
      continue;
    }
    try {
      result.add(_loadFile(skillFile));
    } on FormatException {
      // Skip malformed or oversized SKILL.md files instead of failing the
      // whole catalog; the remaining skills stay usable.
    }
  }
  return result;
}

Skill _loadFile(File file) {
  final size = file.lengthSync();
  if (size > Skill.maxBytes) {
    throw FormatException('${file.path}: SKILL.md is too large');
  }
  final content = file.readAsStringSync();
  final text = content.replaceAll('\r\n', '\n');
  final meta = _parseFrontmatter(file.path, text);
  final name = meta['name'];
  final description = meta['description'];
  if (name == null || name.trim().isEmpty) {
    throw FormatException('${file.path}: skill name is required');
  }
  if (description == null || description.trim().isEmpty) {
    throw FormatException('${file.path}: skill description is required');
  }
  if (description.length > maxSkillDescriptionBytes) {
    throw FormatException('${file.path}: skill description is too large');
  }
  final rawDisabled = meta['disable-model-invocation'];
  final disableModelInvocation = switch (rawDisabled) {
    null => false,
    '' => false,
    'true' => true,
    'false' => false,
    _ => throw FormatException(
      '${file.path}: invalid disable-model-invocation: $rawDisabled',
    ),
  };
  return Skill(
    name: name.trim(),
    description: description.trim(),
    dir: file.parent.path,
    path: file.path,
    content: text,
    disableModelInvocation: disableModelInvocation,
  );
}

Map<String, String> _parseFrontmatter(String path, String text) {
  final lines = text.split('\n');
  if (lines.length < 3 || lines.first.trim() != '---') {
    throw FormatException('$path: missing frontmatter');
  }
  var end = -1;
  for (var i = 1; i < lines.length; i++) {
    if (lines[i].trim() == '---') {
      end = i;
      break;
    }
  }
  if (end == -1) {
    throw FormatException('$path: missing frontmatter terminator');
  }
  final meta = <String, String>{};
  for (final line in lines.sublist(1, end)) {
    final trimmed = line.trim();
    if (trimmed.isEmpty || trimmed.startsWith('#')) {
      continue;
    }
    final colon = trimmed.indexOf(':');
    if (colon <= 0) {
      throw FormatException('$path: invalid frontmatter line "$trimmed"');
    }
    meta[trimmed.substring(0, colon).trim()] = _unquote(
      trimmed.substring(colon + 1).trim(),
    );
  }
  return meta;
}

String _unquote(String value) {
  if (value.isEmpty) {
    return value;
  }
  if (value.startsWith('"')) {
    try {
      return jsonDecode(value) as String;
    } on FormatException {
      // Fall through to the literal value.
    }
  }
  if (value.length >= 2 && value.startsWith("'") && value.endsWith("'")) {
    return value.substring(1, value.length - 1);
  }
  return value;
}

void _validateSummaries(Map<String, Skill> skills) {
  var total = 0;
  for (final skill in skills.values) {
    if (skill.disableModelInvocation) {
      continue;
    }
    total += skill.name.length + skill.description.length;
    if (total > maxSkillSummaryBytes) {
      throw FormatException('skill summaries are too large');
    }
  }
}

import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

export 'package:atlas_runtime/atlas_runtime.dart' show InstructionFile;

/// Loads the global and current-directory AGENTS.md instruction files.
///
/// `~/.atlas/AGENTS.md` is loaded first, then `<cwd>/AGENTS.md`. Missing
/// files are skipped and duplicate paths are loaded once.
List<InstructionFile> loadInstructionFiles({
  String? workingDirectory,
  String? homeDirectory,
}) {
  final home = homeDirectory ?? Platform.environment['HOME'];
  final cwd = workingDirectory ?? Directory.current.path;
  final paths = <String>[
    if (home != null && home.isNotEmpty) '$home/.atlas/AGENTS.md',
    '$cwd/AGENTS.md',
  ];
  final seen = <String>{};
  final result = <InstructionFile>[];
  for (final path in paths) {
    final file = File(path);
    final absolute = file.absolute.path;
    if (!seen.add(absolute)) {
      continue;
    }
    if (!file.existsSync()) {
      continue;
    }
    final content = file.readAsStringSync();
    result.add(
      InstructionFile(
        path: absolute,
        content: content.length > InstructionFile.maxBytes
            ? content.substring(0, InstructionFile.maxBytes)
            : content,
      ),
    );
  }
  return result;
}

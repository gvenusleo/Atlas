import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

import 'file_path.dart';
import 'text_utils.dart';

/// One exact, unique text replacement.
final class TextEdit {
  /// Creates a text edit.
  const TextEdit({required this.oldText, required this.newText});

  /// The exact text to replace.
  final String oldText;

  /// The replacement text.
  final String newText;
}

/// Applies exact, non-overlapping replacements to one text file.
final class EditTool implements Tool {
  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'edit',
    description:
        'Apply one or more exact, unique, non-overlapping text replacements '
        'to a UTF-8 file. Every edit is matched against the original content '
        'and the call is all-or-nothing.',
    inputSchema: {
      'type': 'object',
      'properties': {
        'path': {
          'type': 'string',
          'description':
              'File path, relative to the session working directory or '
              'absolute.',
        },
        'edits': {
          'type': 'array',
          'minItems': 1,
          'description':
              'Exact replacements. Each old_text must occur exactly once in '
              'the original file.',
          'items': {
            'type': 'object',
            'properties': {
              'old_text': {'type': 'string'},
              'new_text': {'type': 'string'},
            },
            'required': ['old_text', 'new_text'],
          },
        },
      },
      'required': ['path', 'edits'],
    },
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    try {
      final path = resolveFilePath(
        context.workingDirectory,
        arguments['path'] as String? ?? '',
      );
      final rawEdits = arguments['edits'];
      if (rawEdits is! List || rawEdits.isEmpty) {
        throw const FormatException('edits must be a non-empty list');
      }
      final edits = <TextEdit>[
        for (final raw in rawEdits)
          if (raw is Map<String, Object?>)
            TextEdit(
              oldText: raw['old_text'] as String? ?? '',
              newText: raw['new_text'] as String? ?? '',
            )
          else
            throw const FormatException('each edit must be an object'),
      ];
      final file = File(path);
      final type = await file.stat().then((stat) => stat.type);
      if (type == FileSystemEntityType.directory) {
        throw FormatException('cannot edit a directory: $path');
      }
      if (type == FileSystemEntityType.notFound) {
        throw FormatException('file not found: $path');
      }
      final bytes = await file.readAsBytes();
      final String decoded;
      try {
        decoded = utf8.decode(bytes);
      } on FormatException {
        throw const FormatException('file is not valid UTF-8');
      }
      final bom = hasUtf8Bom(bytes);
      final newline = primaryLineEnding(decoded);
      final text = normalizeNewlines(decoded);

      final replacements = <({int start, int end, String replacement})>[];
      for (final edit in edits) {
        final oldText = normalizeNewlines(edit.oldText);
        if (oldText.isEmpty) {
          throw const FormatException('old_text must not be empty');
        }
        final first = text.indexOf(oldText);
        if (first == -1) {
          throw FormatException('old_text not found: ${edit.oldText}');
        }
        if (text.indexOf(oldText, first + 1) != -1) {
          throw FormatException(
            'old_text occurs more than once: ${edit.oldText}',
          );
        }
        replacements.add((
          start: first,
          end: first + oldText.length,
          replacement: normalizeNewlines(edit.newText),
        ));
      }
      replacements.sort((a, b) => a.start.compareTo(b.start));
      for (var index = 1; index < replacements.length; index++) {
        if (replacements[index].start < replacements[index - 1].end) {
          throw const FormatException('edits must not overlap');
        }
      }

      var result = text;
      for (final replacement in replacements.reversed) {
        result = result.replaceRange(
          replacement.start,
          replacement.end,
          replacement.replacement,
        );
      }
      result = result.replaceAll('\n', newline);
      final output = bom ? '\uFEFF$result' : result;
      await file.writeAsString(output);
      return ToolResult(
        content: 'Applied ${replacements.length} edit(s) to $path',
      );
    } catch (error) {
      return ToolResult(
        content: error is FormatException ? error.message : '$error',
        isError: true,
      );
    }
  }
}

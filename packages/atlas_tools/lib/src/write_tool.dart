import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

import 'file_path.dart';
import 'text_utils.dart';

/// Creates a file or replaces its complete contents.
final class WriteTool implements Tool {
  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'write',
    description:
        'Create a file or replace its complete contents, creating parent '
        'directories as needed.',
    inputSchema: {
      'type': 'object',
      'properties': {
        'path': {
          'type': 'string',
          'description':
              'File path, relative to the session working directory or '
              'absolute.',
        },
        'content': {
          'type': 'string',
          'description': 'The complete file content.',
        },
      },
      'required': ['path', 'content'],
    },
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    try {
      final path = resolveFilePath(
        context.workingDirectory,
        arguments['path'] as String? ?? '',
      );
      final content = arguments['content'];
      if (content is! String) {
        throw const FormatException('content is required and must be a string');
      }
      final file = File(path);
      await file.parent.create(recursive: true);
      // Read the previous content before overwriting so the result can carry
      // a diff; unreadable files simply report no old text.
      String? oldText;
      if (await file.exists()) {
        try {
          oldText = await file.readAsString();
        } on FileSystemException {
          oldText = null;
        }
      }
      await file.writeAsString(content);
      // Diff metadata is only reported for bounded files.
      final belowLimit =
          content.codeUnits.length <= toolDiffContentLimit &&
          (oldText?.codeUnits.length ?? 0) <= toolDiffContentLimit;
      return ToolResult(
        content: 'Wrote ${utf8Length(content)} bytes to $path',
        metadata: belowLimit
            ? {'path': path, 'newText': content, 'oldText': ?oldText}
            : const {},
      );
    } catch (error) {
      return ToolResult(
        content: error is FormatException ? error.message : '$error',
        isError: true,
      );
    }
  }

  /// Returns the UTF-8 byte length of [value].
  static int utf8Length(String value) => utf8.encode(value).length;
}

import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';

import 'file_path.dart';

/// The maximum number of lines returned in one read.
const readLineLimit = 2000;

/// The maximum number of output bytes returned in one read.
const readByteLimit = 50 * 1024;

/// Reads a bounded range from a UTF-8 text file.
final class ReadTool implements Tool {
  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'read',
    description:
        'Read a bounded range from a UTF-8 text file. Lines are 1-indexed; '
        'use offset and limit to continue through large files.',
    inputSchema: {
      'type': 'object',
      'properties': {
        'path': {
          'type': 'string',
          'description':
              'File path, relative to the session working directory or '
              'absolute.',
        },
        'offset': {
          'type': 'integer',
          'minimum': 1,
          'description':
              'Optional 1-indexed line to start reading from. '
              'Defaults to 1.',
        },
        'limit': {
          'type': 'integer',
          'minimum': 1,
          'maximum': readLineLimit,
          'description': 'Optional maximum number of lines. Defaults to 2000.',
        },
      },
      'required': ['path'],
    },
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    try {
      final path = resolveFilePath(
        context.workingDirectory,
        arguments['path'] as String? ?? '',
      );
      final offset = (arguments['offset'] as num?)?.toInt() ?? 1;
      final limit = (arguments['limit'] as num?)?.toInt() ?? readLineLimit;
      if (offset < 1 || limit < 1 || limit > readLineLimit) {
        throw const FormatException(
          'offset must be >= 1 and limit must be between 1 and 2000',
        );
      }
      final reader = context.fileReader;
      if (reader != null) {
        // Delegate to the ACP client so unsaved editor state is visible;
        // the client returns the requested lines without local byte
        // truncation or a next-offset continuation.
        final result = await reader.readTextFile(
          context.sessionId,
          path: path,
          line: offset,
          limit: limit,
        );
        return ToolResult(content: result.content);
      }
      final file = File(path);
      final type = await file.stat().then((stat) => stat.type);
      if (type == FileSystemEntityType.directory) {
        throw FormatException('cannot read a directory: $path');
      }
      if (type == FileSystemEntityType.notFound) {
        throw FormatException('file not found: $path');
      }
      final String text;
      try {
        text = utf8.decode(await file.readAsBytes());
      } on FormatException {
        throw const FormatException('file is not valid UTF-8');
      }
      final lines = const LineSplitter().convert(text);
      final shown = <String>[];
      var byteCount = 0;
      var lineNumber = offset - 1;
      while (lineNumber < lines.length &&
          shown.length < limit &&
          (shown.isEmpty ||
              byteCount + _lineBytes(lines[lineNumber]) <= readByteLimit)) {
        final line = lines[lineNumber];
        shown.add(line);
        byteCount += _lineBytes(line);
        lineNumber++;
      }
      final nextOffset = lineNumber < lines.length ? lineNumber + 1 : null;
      return ToolResult(
        content: shown.isEmpty
            ? '(empty or no lines in range)'
            : shown.join('\n'),
        metadata: {'next_offset': ?nextOffset},
      );
    } catch (error) {
      return ToolResult(
        content: error is FormatException ? error.message : '$error',
        isError: true,
      );
    }
  }

  static int _lineBytes(String line) => utf8.encode(line).length + 1;
}

/// Shared path resolution for local tools.
library;

import 'dart:io';

/// Resolves [path] against [cwd].
///
/// Rejects empty paths. Absolute paths are cleaned as-is; relative paths are
/// joined to [cwd] (or used alone when [cwd] is empty). `.` and `..`
/// segments are normalized without resolving symbolic links.
String resolveFilePath(String cwd, String path) {
  if (path.trim().isEmpty) {
    throw const FormatException('path is required');
  }
  final file = File(path);
  final combined = file.isAbsolute || cwd.isEmpty ? path : '$cwd/$path';
  return Uri.parse(combined).normalizePath().toFilePath();
}

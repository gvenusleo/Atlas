import 'dart:io';

import '../data/file_browser_service.dart';

/// Immutable row in the visible file tree.
final class const FileBrowserEntry({
  /// Filesystem entry described by this row.
  required final FileSystemEntity entity,

  /// Indentation level below the root.
  required final int depth,

  /// Whether the directory's children are visible.
  required final bool expanded,

  /// Whether the directory is loading.
  required final bool loading,

  /// Last directory loading error, if any.
  final String? error,
});

/// Immutable file tree and preview snapshot published by the controller.
final class FileBrowserState({
  /// Root directory for file operations.
  required final Directory root,
  List<FileBrowserEntry> entries = const [],

  /// Whether the root directory is loading.
  final bool loading = false,

  /// Whether the first root listing has completed successfully.
  final bool loaded = false,

  /// Last root loading error, if any.
  final String? rootError,

  /// File selected for preview, if any.
  final File? selectedFile,

  /// Loaded preview text.
  final String? preview,

  /// Last preview loading error, if any.
  final String? previewError,

  /// Pending copy or cut operation.
  final FileClipboard? clipboard,
}) {
  /// Creates a snapshot with a protected row list.
  this : entries = List.unmodifiable(entries);

  /// Visible rows in display order.
  final List<FileBrowserEntry> entries;
}

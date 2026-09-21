import 'dart:io';

import '../data/file_browser_service.dart';

/// Immutable row in the visible file tree.
final class FileBrowserEntry {
  /// Creates a row snapshot.
  const FileBrowserEntry({
    required this.entity,
    required this.depth,
    required this.expanded,
    required this.loading,
    this.error,
  });

  /// Filesystem entry described by this row.
  final FileSystemEntity entity;

  /// Indentation level below the root.
  final int depth;

  /// Whether the directory's children are visible.
  final bool expanded;

  /// Whether the directory is loading.
  final bool loading;

  /// Last directory loading error, if any.
  final String? error;
}

/// Immutable file tree and preview snapshot published by the controller.
final class FileBrowserState {
  /// Creates a snapshot with a protected row list.
  FileBrowserState({
    required this.root,
    List<FileBrowserEntry> entries = const [],
    this.loading = false,
    this.loaded = false,
    this.rootError,
    this.selectedFile,
    this.preview,
    this.previewError,
    this.clipboard,
  }) : entries = List.unmodifiable(entries);

  /// Root directory for file operations.
  final Directory root;

  /// Visible rows in display order.
  final List<FileBrowserEntry> entries;

  /// Whether the root directory is loading.
  final bool loading;

  /// Whether the first root listing has completed successfully.
  final bool loaded;

  /// Last root loading error, if any.
  final String? rootError;

  /// File selected for preview, if any.
  final File? selectedFile;

  /// Loaded preview text.
  final String? preview;

  /// Last preview loading error, if any.
  final String? previewError;

  /// Pending copy or cut operation.
  final FileClipboard? clipboard;
}

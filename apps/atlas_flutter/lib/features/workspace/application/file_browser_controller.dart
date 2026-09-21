import 'dart:async';
import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/file_browser_service.dart';
import 'file_browser_state.dart';

/// Owns a browser's cached tree, filesystem subscriptions, and file commands.
final class FileBrowserController extends Notifier<FileBrowserState> {
  /// Creates a browser over the injected filesystem adapter.
  FileBrowserController({
    required this.workingDirectory,
    required this.service,
  });

  /// Root path for this browser's lifetime.
  final String workingDirectory;

  /// Adapter for all filesystem access, including watches.
  final FileBrowserService service;

  static const _reloadDebounce = Duration(milliseconds: 300);
  late _TreeNode _root;
  final _watchers = <String, StreamSubscription<FileSystemEvent>>{};
  final _debounce = <String, Timer>{};
  Timer? _previewDebounce;
  File? _selectedFile;
  String? _preview;
  String? _previewError;
  FileClipboard? _clipboard;
  var _previewVersion = 0;

  @override
  FileBrowserState build() {
    _root = _TreeNode(Directory(workingDirectory).absolute, -1)
      ..expanded = true;
    ref.onDispose(() {
      _previewVersion++;
      _previewDebounce?.cancel();
      for (final timer in _debounce.values) {
        timer.cancel();
      }
      for (final subscription in _watchers.values) {
        unawaited(subscription.cancel());
      }
      _debounce.clear();
      _watchers.clear();
    });
    unawaited(
      Future<void>.microtask(() {
        if (ref.mounted) return _loadChildren(_root);
      }),
    );
    return FileBrowserState(root: _root.entity as Directory, loading: true);
  }

  /// Expands or collapses a row, reusing already loaded children.
  Future<void> toggle(FileBrowserEntry entry) async {
    final node = _findNode(entry.entity.path);
    if (node == null) return;
    node.expanded = !node.expanded;
    if (node.expanded && node.children == null) await _loadChildren(node);
    _publish();
  }

  /// Reloads all expanded directories.
  Future<void> refresh() => _reloadExpanded(_root);

  Future<void> _reloadExpanded(_TreeNode node) async {
    if (!ref.mounted || !node.expanded) return;
    await _loadChildren(node);
    for (final child in node.children ?? const <_TreeNode>[]) {
      await _reloadExpanded(child);
    }
  }

  Future<void> _loadChildren(_TreeNode node, {bool auto = false}) async {
    if (!ref.mounted) return;
    final version = ++node.version;
    if (!auto) {
      node.loading = true;
      _publish();
    }
    try {
      final entries = await service.listDirectory(node.entity as Directory);
      if (!ref.mounted || node.version != version) return;
      final existing = {
        for (final child in node.children ?? const <_TreeNode>[])
          child.entity.path: child,
      };
      node.children = [
        for (final entry in entries)
          if (existing[entry.path] case final previous?
              when (previous.entity is Directory) == (entry is Directory))
            previous
          else
            _TreeNode(entry, node.depth + 1),
      ];
      node.error = null;
      _watchDirectory(node.entity.path);
    } on FileSystemException catch (error) {
      if (!ref.mounted || node.version != version) return;
      node.error = error.message;
    } finally {
      if (ref.mounted && node.version == version) {
        node.loading = false;
        _pruneWatchers();
        _publish();
      }
    }
  }

  void _watchDirectory(String path) {
    if (!ref.mounted || _watchers.containsKey(path)) return;
    try {
      _watchers[path] = service
          .watchDirectory(path)
          .listen(
            (event) {
              if (event.path == path) {
                _unwatch(path);
                return;
              }
              if (_selectedFile?.path == event.path) {
                _previewDebounce?.cancel();
                final selected = _selectedFile;
                _previewDebounce = Timer(_reloadDebounce, () {
                  if (ref.mounted && _selectedFile == selected) {
                    unawaited(_loadPreview());
                  }
                });
              }
              _debounce[path]?.cancel();
              _debounce[path] = Timer(_reloadDebounce, () {
                _debounce.remove(path);
                final node = _findNode(path);
                if (node == null) {
                  _unwatch(path);
                } else if (ref.mounted) {
                  unawaited(_loadChildren(node, auto: true));
                }
              });
            },
            onError: (Object _) => _unwatch(path),
            cancelOnError: true,
          );
    } on Object {
      // Unsupported watches retain manual refresh as a fallback.
    }
  }

  void _unwatch(String path) {
    _debounce.remove(path)?.cancel();
    unawaited(_watchers.remove(path)?.cancel());
  }

  void _pruneWatchers() {
    for (final path in _watchers.keys.toList()) {
      if (_findNode(path) == null) _unwatch(path);
    }
  }

  /// Opens a file preview, ignoring results superseded by another selection.
  Future<void> openFile(File file) async {
    _selectedFile = file;
    _preview = null;
    _previewError = null;
    _publish();
    await _loadPreview();
  }

  /// Returns from preview to the tree.
  void closePreview() {
    _previewVersion++;
    _previewDebounce?.cancel();
    _selectedFile = null;
    _preview = null;
    _previewError = null;
    _publish();
  }

  Future<void> _loadPreview() async {
    final file = _selectedFile;
    if (file == null || !ref.mounted) return;
    final version = ++_previewVersion;
    _previewError = null;
    _publish();
    try {
      final preview = await service.readPreview(file);
      if (!ref.mounted || version != _previewVersion) return;
      _preview = preview;
    } on FileSystemException catch (error) {
      final exists = await service.fileExists(file);
      if (!ref.mounted || version != _previewVersion) return;
      _previewError = exists ? error.message : 'This file no longer exists.';
    } on FormatException catch (error) {
      if (!ref.mounted || version != _previewVersion) return;
      _previewError = error.message;
    }
    _publish();
  }

  /// Stages an entry for copy or cut.
  void copy(FileSystemEntity entity, {required bool cut}) {
    _clipboard = FileClipboard(path: entity.path, cut: cut);
    _publish();
  }

  /// Pastes the staged entry and refreshes its destination.
  Future<void> pasteInto(Directory directory) async {
    final clip = _clipboard;
    if (clip == null) return;
    final source = await service.entityAt(clip.path);
    if (!ref.mounted) return;
    if (clip.cut) {
      await service.moveInto(
        source: source,
        directory: directory,
        root: _root.entity.path,
      );
      if (!ref.mounted) return;
      if (identical(_clipboard, clip)) _clipboard = null;
    } else {
      await service.copyInto(
        source: source,
        directory: directory,
        root: _root.entity.path,
      );
    }
    await _reloadAfterWrite(directory.path, expand: true);
  }

  /// Creates a file and opens its preview after reloading the parent.
  Future<void> createFile(Directory directory, String name) async {
    final file = await service.createFile(
      directory: directory,
      name: name,
      root: _root.entity.path,
    );
    if (!ref.mounted) return;
    await _reloadAfterWrite(directory.path, expand: true);
    if (ref.mounted) await openFile(file);
  }

  /// Creates a directory and refreshes its parent.
  Future<void> createFolder(Directory directory, String name) async {
    await service.createDirectory(
      directory: directory,
      name: name,
      root: _root.entity.path,
    );
    await _reloadAfterWrite(directory.path, expand: true);
  }

  /// Renames an entry and keeps its preview in sync.
  Future<void> rename(FileSystemEntity entity, String name) async {
    final renamed = await service.rename(
      entity: entity,
      name: name,
      root: _root.entity.path,
    );
    if (!ref.mounted) return;
    if (_selectedFile?.path == entity.path) {
      if (renamed is File) {
        await openFile(renamed);
      } else {
        closePreview();
      }
    }
    await _reloadAfterWrite(entity.parent.path);
  }

  /// Trashes a user-confirmed entry and clears affected preview and clipboard.
  Future<void> trash(FileSystemEntity entity) async {
    await service.trashPath(entity.path, _root.entity.path);
    if (!ref.mounted) return;
    if (_selectedFile?.path == entity.path ||
        (_selectedFile?.path.startsWith(
              '${entity.path}${Platform.pathSeparator}',
            ) ??
            false)) {
      closePreview();
    }
    if (_clipboard?.path == entity.path) _clipboard = null;
    await _reloadAfterWrite(entity.parent.path);
  }

  /// Reveals an entry in the platform file manager.
  Future<void> reveal(String path) =>
      service.revealPath(path, _root.entity.path);

  Future<void> _reloadAfterWrite(String path, {bool expand = false}) async {
    if (!ref.mounted) return;
    final node = _findNode(path);
    if (node == null) {
      await refresh();
    } else {
      if (expand) node.expanded = true;
      await _loadChildren(node);
    }
  }

  _TreeNode? _findNode(String path) {
    _TreeNode? visit(_TreeNode node) {
      if (node.entity.path == path) return node;
      for (final child in node.children ?? const <_TreeNode>[]) {
        final match = visit(child);
        if (match != null) return match;
      }
      return null;
    }

    return visit(_root);
  }

  void _publish() {
    if (!ref.mounted) return;
    final entries = <FileBrowserEntry>[];
    void visit(_TreeNode node) {
      entries.add(
        FileBrowserEntry(
          entity: node.entity,
          depth: node.depth,
          expanded: node.expanded,
          loading: node.loading,
          error: node.error,
        ),
      );
      if (node.expanded) {
        for (final child in node.children ?? const <_TreeNode>[]) {
          visit(child);
        }
      }
    }

    for (final child in _root.children ?? const <_TreeNode>[]) {
      visit(child);
    }
    state = FileBrowserState(
      root: _root.entity as Directory,
      entries: entries,
      loading: _root.loading,
      loaded: _root.children != null,
      rootError: _root.error,
      selectedFile: _selectedFile,
      preview: _preview,
      previewError: _previewError,
      clipboard: _clipboard,
    );
  }
}

class _TreeNode {
  _TreeNode(this.entity, this.depth);

  final FileSystemEntity entity;
  final int depth;
  bool expanded = false;
  bool loading = false;
  int version = 0;
  String? error;
  List<_TreeNode>? children;
}

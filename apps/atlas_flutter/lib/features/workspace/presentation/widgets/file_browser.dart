import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';

const _maximumPreviewBytes = 512 * 1024;
const _maximumEntriesPerFolder = 500;

/// Browses a working directory as a lazy-loading file tree and previews
/// UTF-8 text files.
class FileBrowser extends StatefulWidget {
  /// Creates a browser rooted at [workingDirectory].
  const FileBrowser({super.key, required this.workingDirectory});

  /// Directory that users cannot navigate above.
  final String workingDirectory;

  @override
  State<FileBrowser> createState() => _FileBrowserState();
}

/// A directory node in the lazy-loading file tree.
class _TreeNode {
  _TreeNode({required this.entity, required this.depth});

  final FileSystemEntity entity;
  final int depth;
  bool expanded = false;

  /// Children once loaded; null while not yet read.
  List<_TreeNode>? children;
  bool loading = false;
  String? error;
}

class _FileBrowserState extends State<FileBrowser> {
  late Directory _root;
  late _TreeNode _rootNode;
  List<_TreeNode> _visibleNodes = const [];
  File? _selectedFile;
  String? _preview;
  String? _error;

  @override
  void initState() {
    super.initState();
    _resetRoot();
  }

  @override
  void didUpdateWidget(covariant FileBrowser oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.workingDirectory != widget.workingDirectory) {
      _resetRoot();
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: 38,
          child: _selectedFile == null
              ? _buildToolbar(colors)
              : _buildPreviewToolbar(colors),
        ),
        const Divider(),
        Expanded(child: _buildContent(colors)),
      ],
    );
  }

  Widget _buildToolbar(AtlasColors colors) {
    return Row(
      children: [
        Expanded(
          child: Padding(
            padding: const EdgeInsets.only(left: 10),
            child: Text(
              _root.path,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: colors.textSecondary, fontSize: 11.5),
            ),
          ),
        ),
        _FileAction(
          icon: LucideIcons.refreshCw,
          tooltip: 'Refresh files',
          onPressed: _refresh,
        ),
      ],
    );
  }

  Widget _buildPreviewToolbar(AtlasColors colors) {
    return Row(
      children: [
        _FileAction(
          icon: LucideIcons.arrowLeft,
          tooltip: 'Back to files',
          onPressed: _closePreview,
        ),
        Expanded(
          child: Padding(
            padding: const EdgeInsets.only(left: 4),
            child: Text(
              _selectedFile?.path.split(Platform.pathSeparator).last ?? '',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 11.5,
                fontFamily: 'monospace',
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildContent(AtlasColors colors) {
    if (_selectedFile != null) {
      if (_error != null) {
        return Padding(
          padding: const EdgeInsets.all(14),
          child: Text(
            _error!,
            style: TextStyle(color: colors.error, fontSize: 12, height: 1.45),
          ),
        );
      }
      return SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 20),
        child: SelectableText(
          _preview ?? '',
          style: TextStyle(
            color: colors.textPrimary,
            fontFamily: WorkspaceMetrics.monospaceFontFamily,
            fontSize: 11.5,
            height: 1.45,
          ),
        ),
      );
    }
    if (_rootNode.loading && _rootNode.children == null) {
      return Center(
        child: SizedBox.square(
          dimension: 18,
          child: CircularProgressIndicator(
            strokeWidth: 1.5,
            color: colors.accent,
          ),
        ),
      );
    }
    if (_rootNode.error != null && _rootNode.children == null) {
      return Padding(
        padding: const EdgeInsets.all(14),
        child: Text(
          _rootNode.error!,
          style: TextStyle(color: colors.error, fontSize: 12, height: 1.45),
        ),
      );
    }
    if (_rootNode.children?.isEmpty ?? false) {
      return Center(
        child: Text(
          'Empty folder',
          style: TextStyle(color: colors.textSecondary, fontSize: 12),
        ),
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.symmetric(vertical: 4),
      itemCount: _visibleNodes.length,
      itemBuilder: (context, index) =>
          _buildRow(_visibleNodes[index], colors),
    );
  }

  Widget _buildRow(_TreeNode node, AtlasColors colors) {
    final isDirectory = node.entity is Directory;
    return InkWell(
      onTap: () => isDirectory
          ? _toggleNode(node)
          : _openFile(node.entity as File),
      child: SizedBox(
        height: 30,
        child: Padding(
          padding: EdgeInsets.only(left: 6.0 + node.depth * 12.0, right: 8),
          child: Row(
            children: [
              Icon(
                isDirectory
                    ? (node.expanded
                        ? LucideIcons.folderOpen
                        : LucideIcons.folder)
                    : LucideIcons.file,
                size: 15,
                color: colors.textPrimary,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  node.entity.path.split(Platform.pathSeparator).last,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: colors.textPrimary, fontSize: 12),
                ),
              ),
              if (node.loading)
                SizedBox.square(
                  dimension: 12,
                  child: CircularProgressIndicator(
                    strokeWidth: 1.5,
                    color: colors.accent,
                  ),
                )
              else if (node.error != null)
                Tooltip(
                  message: node.error!,
                  child: Icon(
                    LucideIcons.triangleAlert,
                    size: 14,
                    color: colors.error,
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  void _resetRoot() {
    _root = Directory(widget.workingDirectory).absolute;
    _rootNode = _TreeNode(entity: _root, depth: 0)..expanded = true;
    _visibleNodes = const [];
    _selectedFile = null;
    _preview = null;
    _error = null;
    unawaited(_loadChildren(_rootNode));
  }

  Future<void> _toggleNode(_TreeNode node) async {
    if (node.expanded) {
      node.expanded = false;
      _rebuildVisible();
      return;
    }
    node.expanded = true;
    if (node.children == null) {
      await _loadChildren(node);
    }
    _rebuildVisible();
  }

  Future<void> _loadChildren(_TreeNode node) async {
    node.loading = true;
    _rebuildVisible();
    try {
      final entries = await (node.entity as Directory)
          .list(followLinks: false)
          .take(_maximumEntriesPerFolder)
          .toList();
      entries.sort((a, b) {
        final directoryOrder =
            (a is Directory ? 0 : 1) - (b is Directory ? 0 : 1);
        if (directoryOrder != 0) {
          return directoryOrder;
        }
        return a.path.toLowerCase().compareTo(b.path.toLowerCase());
      });
      // Reuse existing child nodes by path so expanded state survives reloads.
      final existing = {
        for (final child in node.children ?? const <_TreeNode>[])
          child.entity.path: child,
      };
      final childDepth = node == _rootNode ? 0 : node.depth + 1;
      node.children = entries
          .map((entry) =>
              existing[entry.path] ??
              _TreeNode(entity: entry, depth: childDepth))
          .toList();
      node.error = null;
    } on FileSystemException catch (error) {
      node.error = error.message;
    } finally {
      node.loading = false;
      if (mounted) {
        _rebuildVisible();
      }
    }
  }

  /// Reloads every expanded folder so newly created files appear.
  Future<void> _refresh() async {
    await _reloadExpanded(_rootNode);
  }

  /// Reloads [node] and, when expanded, its expanded descendants.
  Future<void> _reloadExpanded(_TreeNode node) async {
    if (!node.expanded) {
      return;
    }
    await _loadChildren(node);
    for (final child in node.children ?? const <_TreeNode>[]) {
      await _reloadExpanded(child);
    }
  }

  void _rebuildVisible() {
    final nodes = <_TreeNode>[];
    void visit(_TreeNode node) {
      nodes.add(node);
      if (node.expanded && node.children != null) {
        for (final child in node.children!) {
          visit(child);
        }
      }
    }

    // The root directory itself is not shown; its children are the top level.
    for (final child in _rootNode.children ?? const <_TreeNode>[]) {
      visit(child);
    }
    setState(() => _visibleNodes = nodes);
  }

  void _openFile(File file) {
    setState(() {
      _selectedFile = file;
      _preview = null;
      _error = null;
    });
    _loadPreview();
  }

  void _closePreview() {
    setState(() {
      _selectedFile = null;
      _preview = null;
      _error = null;
    });
  }

  Future<void> _loadPreview() async {
    final file = _selectedFile;
    if (file == null) {
      return;
    }
    setState(() {
      _error = null;
    });
    try {
      final length = await file.length();
      if (length > _maximumPreviewBytes) {
        throw const FormatException(
          'File is larger than the 512 KB preview limit.',
        );
      }
      final bytes = await file.readAsBytes();
      if (bytes.contains(0)) {
        throw const FormatException('Binary files cannot be previewed.');
      }
      final preview = utf8.decode(bytes);
      if (mounted) {
        setState(() => _preview = preview);
      }
    } on FileSystemException catch (error) {
      if (mounted) {
        setState(() => _error = error.message);
      }
    } on FormatException catch (error) {
      if (mounted) {
        setState(() => _error = error.message);
      }
    }
  }
}

class _FileAction extends StatelessWidget {
  const _FileAction({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
  });

  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Tooltip(
      message: tooltip,
      child: IconButton(
        padding: EdgeInsets.zero,
        constraints: const BoxConstraints.tightFor(width: 38, height: 38),
        onPressed: onPressed,
        icon: Icon(
          icon,
          size: 15,
          color: colors.textSecondary,
        ),
      ),
    );
  }
}

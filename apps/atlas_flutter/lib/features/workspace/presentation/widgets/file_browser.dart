import 'dart:async';
import 'dart:io';

import 'package:atlas_flutter/features/workspace/presentation/widgets/workspace_controls.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../../data/file_browser_service.dart';
import '../workspace_metrics.dart';

/// Keeps one [FileBrowser] per session so expand/preview state survives focus changes.
class FileBrowserHost extends ConsumerStatefulWidget {
  /// Creates a host for the focused session's file tree.
  const FileBrowserHost({
    super.key,
    required this.sessionKey,
    required this.workingDirectory,
    this.service = const FileBrowserService(),
  });

  /// Cache key of the focused session or draft.
  final String sessionKey;

  /// Working directory of the focused session.
  final String workingDirectory;

  /// Filesystem adapter used by each browser.
  final FileBrowserService service;

  @override
  ConsumerState<FileBrowserHost> createState() => _FileBrowserHostState();
}

class _FileBrowserHostState extends ConsumerState<FileBrowserHost> {
  final _directories = <String, String>{};

  @override
  Widget build(BuildContext context) {
    final liveKeys = ref.watch(
      workspaceProvider.select((state) => state.workspaces.keys.toSet()),
    );
    final directories = <String, String>{
      for (final entry in _directories.entries)
        if (liveKeys.contains(entry.key)) entry.key: entry.value,
      widget.sessionKey: widget.workingDirectory,
    };
    _directories
      ..clear()
      ..addAll(directories);
    final keys = directories.keys.toList();
    return IndexedStack(
      index: keys.indexOf(widget.sessionKey),
      children: [
        for (final key in keys)
          FileBrowser(
            key: ValueKey('files-$key'),
            workingDirectory: directories[key]!,
            service: widget.service,
          ),
      ],
    );
  }
}

/// Browses a working directory as a lazy-loading file tree and previews
/// UTF-8 text files.
class FileBrowser extends StatefulWidget {
  /// Creates a browser rooted at [workingDirectory].
  const FileBrowser({
    super.key,
    required this.workingDirectory,
    this.service = const FileBrowserService(),
  });

  /// Directory that users cannot navigate above.
  final String workingDirectory;

  /// Filesystem adapter used by the browser.
  final FileBrowserService service;

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
        Stack(
          children: [
            Transform.translate(
              offset: const Offset(-4, 0),
              child: const Divider(),
            ),
            Positioned(
              right: 0,
              child: Container(height: 1, width: 4, color: colors.divider),
            ),
          ],
        ),
        Expanded(child: _buildContent(colors)),
      ],
    );
  }

  Widget _buildToolbar(AtlasColors colors) {
    return Row(
      children: [
        Expanded(
          child: Padding(
            padding: const EdgeInsets.only(left: 4),
            child: Text(
              '../${_root.path.split(Platform.pathSeparator).last}',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: colors.textSecondary, fontSize: 11.5),
            ),
          ),
        ),
        WorkspaceToolbarButton(
          icon: LucideIcons.refreshCw,
          tooltip: 'Refresh files',
          onPressed: _refresh,
        ),
        const SizedBox(width: 6),
      ],
    );
  }

  Widget _buildPreviewToolbar(AtlasColors colors) {
    final selected = _selectedFile;
    return Row(
      children: [
        Expanded(
          child: Padding(
            padding: const EdgeInsets.only(left: 4),
            child: Text(
              selected == null ? '' : _relativePath(selected.path),
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
        WorkspaceToolbarButton(
          icon: LucideIcons.x,
          tooltip: 'Back to files',
          onPressed: _closePreview,
        ),
        const SizedBox(width: 6),
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
        padding: const EdgeInsets.fromLTRB(6, 6, 6, 12),
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
      padding: const EdgeInsets.fromLTRB(2, 6, 6, 6),
      itemCount: _visibleNodes.length,
      itemBuilder: (context, index) => _buildRow(_visibleNodes[index], colors),
    );
  }

  Widget _buildRow(_TreeNode node, AtlasColors colors) {
    final isDirectory = node.entity is Directory;
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: () =>
            isDirectory ? _toggleNode(node) : _openFile(node.entity as File),
        child: SizedBox(
          height: 26,
          child: Padding(
            padding: const EdgeInsets.only(left: 6, right: 8),
            child: Row(
              children: [
                // One guide line per ancestor level, running the full row.
                for (var depth = 0; depth < node.depth; depth++)
                  SizedBox(
                    width: 12,
                    height: double.infinity,
                    child: CustomPaint(
                      painter: _GuideLinePainter(
                        color: colors.textSecondary.withValues(alpha: 0.45),
                      ),
                    ),
                  ),
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
      ),
    );
  }

  /// Returns [path] relative to the browser root directory for display.
  String _relativePath(String path) {
    final rootSegments = _root.path.split(Platform.pathSeparator);
    final pathSegments = path.split(Platform.pathSeparator);
    var common = 0;
    while (common < rootSegments.length &&
        common < pathSegments.length &&
        rootSegments[common] == pathSegments[common]) {
      common++;
    }
    final up = List.filled(rootSegments.length - common, '..');
    return [
      ...up,
      ...pathSegments.sublist(common),
    ].join(Platform.pathSeparator);
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
      final entries = await widget.service.listDirectory(
        node.entity as Directory,
      );
      // Reuse existing child nodes by path so expanded state survives reloads.
      final existing = {
        for (final child in node.children ?? const <_TreeNode>[])
          child.entity.path: child,
      };
      final childDepth = node == _rootNode ? 0 : node.depth + 1;
      node.children = entries
          .map(
            (entry) =>
                existing[entry.path] ??
                _TreeNode(entity: entry, depth: childDepth),
          )
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
      final preview = await widget.service.readPreview(file);
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

/// Paints a vertical tree guide line spanning the full row height.
class _GuideLinePainter extends CustomPainter {
  const _GuideLinePainter({required this.color});

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    final x = size.width / 2;
    canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);
  }

  @override
  bool shouldRepaint(covariant _GuideLinePainter oldDelegate) =>
      oldDelegate.color != color;
}

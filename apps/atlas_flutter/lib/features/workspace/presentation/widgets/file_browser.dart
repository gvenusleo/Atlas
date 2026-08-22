import 'dart:async';
import 'dart:io';

import 'package:atlas_flutter/features/workspace/presentation/widgets/workspace_controls.dart';
import 'package:clipboard/clipboard.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/markdown/atlas_markdown.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../../data/file_browser_service.dart';
import '../workspace_metrics.dart';
import 'file_browser_menu.dart';

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
  var _markdownPreview = false;
  FileClipboard? _clipboard;
  final _rootMenu = MenuController();
  final _rowMenus = <MenuController>[];

  void _dismissMenus() {
    if (_rootMenu.isOpen) {
      _rootMenu.close();
    }
    for (final menu in _rowMenus) {
      if (menu.isOpen) {
        menu.close();
      }
    }
  }

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
    final markdown = selected != null && _isMarkdownFile(selected);
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
        if (markdown) ...[
          WorkspaceToolbarButton(
            icon: _markdownPreview
                ? LucideIcons.fileText
                : LucideIcons.bookOpenText,
            tooltip: 'Toggle markdown preview',
            onPressed: () {
              if (_preview == null) {
                return;
              }
              setState(() => _markdownPreview = !_markdownPreview);
            },
          ),
          const SizedBox(width: 4),
        ],
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
      if (_markdownPreview && _isMarkdownFile(_selectedFile!)) {
        return SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(12, 6, 12, 18),
          child: SelectionArea(
            child: AtlasMarkdown(
              data: _preview ?? '',
              fontFamily: WorkspaceMetrics.monospaceFontFamily,
            ),
          ),
        );
      }
      return SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(12, 6, 12, 18),
        child: SelectableText(
          _preview ?? '',
          style: TextStyle(
            color: colors.textPrimary,
            fontFamily: WorkspaceMetrics.monospaceFontFamily,
            fontSize: 14,
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
    final empty = _rootNode.children?.isEmpty ?? false;
    return Stack(
      children: [
        MenuAnchor(
          controller: _rootMenu,
          consumeOutsideTap: true,
          menuChildren: _rootMenuItems(colors),
          child: const SizedBox.shrink(),
        ),
        Positioned.fill(
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onSecondaryTapUp: (details) {
              _dismissMenus();
              _rootMenu.open(position: details.localPosition);
            },
            child: empty
                ? Center(
                    child: Text(
                      'Empty folder',
                      style: TextStyle(
                        color: colors.textSecondary,
                        fontSize: 12,
                      ),
                    ),
                  )
                : null,
          ),
        ),
        if (!empty)
          ListView.builder(
            padding: const EdgeInsets.fromLTRB(2, 6, 6, 6),
            itemCount: _visibleNodes.length,
            itemBuilder: (context, index) =>
                _buildRow(_visibleNodes[index], colors),
          ),
      ],
    );
  }

  Widget _buildRow(_TreeNode node, AtlasColors colors) {
    final isDirectory = node.entity is Directory;
    return FileRowMenu(
      registry: _rowMenus,
      onOpen: _dismissMenus,
      items: isDirectory
          ? _folderMenuItems(node, colors)
          : _fileMenuItems(node, colors),
      child: WorkspaceHoverSurface(
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: () {
            _dismissMenus();
            if (isDirectory) {
              _toggleNode(node);
            } else {
              _openFile(node.entity as File);
            }
          },
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
      _markdownPreview = false;
    });
    _loadPreview();
  }

  void _closePreview() {
    setState(() {
      _selectedFile = null;
      _preview = null;
      _error = null;
      _markdownPreview = false;
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

  List<Widget> _rootMenuItems(AtlasColors colors) => fileBrowserRootMenu(
    colors: colors,
    canPaste: _clipboard != null,
    actions: _menuActions(directory: _root),
  );

  List<Widget> _fileMenuItems(_TreeNode node, AtlasColors colors) =>
      fileBrowserFileMenu(
        colors: colors,
        actions: _menuActions(entity: node.entity, node: node),
      );

  List<Widget> _folderMenuItems(_TreeNode node, AtlasColors colors) =>
      fileBrowserFolderMenu(
        colors: colors,
        canPaste: _clipboard != null,
        actions: _menuActions(
          entity: node.entity,
          node: node,
          directory: node.entity as Directory,
        ),
      );

  FileBrowserMenuActions _menuActions({
    FileSystemEntity? entity,
    _TreeNode? node,
    Directory? directory,
  }) {
    final target = directory ?? _root;
    return FileBrowserMenuActions(
      onNewFile: () => unawaited(_createFile(target, parent: node)),
      onNewFolder: () => unawaited(_createFolder(target, parent: node)),
      onCopy: () {
        if (entity != null) {
          _copy(entity, cut: false);
        }
      },
      onCut: () {
        if (entity != null) {
          _copy(entity, cut: true);
        }
      },
      onPaste: () => unawaited(_pasteInto(target, parent: node)),
      onCopyPath: () {
        if (entity != null) {
          unawaited(_copyPath(entity.path));
        }
      },
      onCopyRelativePath: () {
        if (entity != null) {
          unawaited(_copyPath(_relativePath(entity.path)));
        }
      },
      onRename: () {
        if (node != null) {
          unawaited(_rename(node));
        }
      },
      onReveal: () {
        if (entity != null) {
          unawaited(_reveal(entity.path));
        }
      },
      onTrash: () {
        if (node != null) {
          unawaited(_trash(node));
        }
      },
    );
  }

  void _copy(FileSystemEntity entity, {required bool cut}) {
    setState(() => _clipboard = FileClipboard(path: entity.path, cut: cut));
  }

  Future<void> _copyPath(String path) async {
    await FlutterClipboard.copy(path);
  }

  Future<void> _pasteInto(Directory directory, {_TreeNode? parent}) async {
    final clip = _clipboard;
    if (clip == null) {
      return;
    }
    final source =
        FileSystemEntity.typeSync(clip.path) == FileSystemEntityType.directory
        ? Directory(clip.path)
        : File(clip.path);
    try {
      if (clip.cut) {
        await widget.service.moveInto(
          source: source,
          directory: directory,
          root: _root.path,
        );
        if (mounted) {
          setState(() => _clipboard = null);
        }
      } else {
        await widget.service.copyInto(
          source: source,
          directory: directory,
          root: _root.path,
        );
      }
      await _reloadAfterWrite(parent);
    } on FileSystemException catch (error) {
      _showNotice(error.message);
    }
  }

  Future<void> _createFile(Directory directory, {_TreeNode? parent}) async {
    final name = await promptFileName(
      context,
      title: 'New File',
      hint: 'Name',
      initial: 'untitled.md',
    );
    if (name == null) {
      return;
    }
    try {
      final file = await widget.service.createFile(
        directory: directory,
        name: name,
        root: _root.path,
      );
      await _reloadAfterWrite(parent);
      if (mounted) {
        _openFile(file);
      }
    } on FileSystemException catch (error) {
      _showNotice(error.message);
    }
  }

  Future<void> _createFolder(Directory directory, {_TreeNode? parent}) async {
    final name = await promptFileName(
      context,
      title: 'New Folder',
      hint: 'Name',
      initial: 'untitled',
    );
    if (name == null) {
      return;
    }
    try {
      await widget.service.createDirectory(
        directory: directory,
        name: name,
        root: _root.path,
      );
      await _reloadAfterWrite(parent);
    } on FileSystemException catch (error) {
      _showNotice(error.message);
    }
  }

  Future<void> _rename(_TreeNode node) async {
    final current = node.entity.path.split(Platform.pathSeparator).last;
    final name = await promptFileName(
      context,
      title: 'Rename',
      hint: 'Name',
      initial: current,
    );
    if (name == null || name == current) {
      return;
    }
    try {
      final renamed = await widget.service.rename(
        entity: node.entity,
        name: name,
        root: _root.path,
      );
      if (_selectedFile?.path == node.entity.path) {
        if (renamed is File) {
          _openFile(renamed);
        } else {
          _closePreview();
        }
      }
      await _reloadParent(node);
    } on FileSystemException catch (error) {
      _showNotice(error.message);
    }
  }

  Future<void> _trash(_TreeNode node) async {
    final name = node.entity.path.split(Platform.pathSeparator).last;
    final confirmed = await confirmMoveToTrash(context, name);
    if (!confirmed) {
      return;
    }
    try {
      await widget.service.trashPath(node.entity.path, _root.path);
      if (_selectedFile?.path == node.entity.path ||
          (_selectedFile != null &&
              _selectedFile!.path.startsWith(
                '${node.entity.path}${Platform.pathSeparator}',
              ))) {
        _closePreview();
      }
      if (_clipboard?.path == node.entity.path) {
        setState(() => _clipboard = null);
      }
      await _reloadParent(node);
    } on FileSystemException catch (error) {
      _showNotice(error.message);
    }
  }

  Future<void> _reveal(String path) async {
    try {
      await widget.service.revealPath(path, _root.path);
    } on FileSystemException catch (error) {
      _showNotice(error.message);
    }
  }

  Future<void> _reloadAfterWrite(_TreeNode? parent) async {
    if (parent == null) {
      await _loadChildren(_rootNode);
      return;
    }
    parent.expanded = true;
    await _loadChildren(parent);
  }

  Future<void> _reloadParent(_TreeNode node) async {
    final parentPath = node.entity.parent.path;
    if (parentPath == _root.path) {
      await _loadChildren(_rootNode);
      return;
    }
    final parent = _findNode(parentPath);
    if (parent != null) {
      await _loadChildren(parent);
    } else {
      await _refresh();
    }
  }

  _TreeNode? _findNode(String path) {
    _TreeNode? visit(_TreeNode node) {
      if (node.entity.path == path) {
        return node;
      }
      for (final child in node.children ?? const <_TreeNode>[]) {
        final match = visit(child);
        if (match != null) {
          return match;
        }
      }
      return null;
    }

    return visit(_rootNode);
  }

  void _showNotice(String message) {
    if (!mounted) {
      return;
    }
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(message)));
  }
}

/// Whether [file] is a Markdown document that can be previewed.
bool _isMarkdownFile(File file) {
  final extension = file.path.split('.').last.toLowerCase();
  return extension == 'md' || extension == 'markdown';
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

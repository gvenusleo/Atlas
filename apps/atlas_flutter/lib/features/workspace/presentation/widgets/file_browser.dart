import 'dart:async';
import 'dart:io';

import 'package:clipboard/clipboard.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/markdown/atlas_markdown.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/file_browser_controller.dart';
import '../../application/file_browser_state.dart';
import '../../application/workspace_controller.dart';
import '../../data/file_browser_service.dart';
import '../workspace_metrics.dart';
import 'file_browser_menu.dart';
import 'workspace_controls.dart';

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

/// Renders a lazy file tree and text preview controlled by Riverpod.
class FileBrowser extends ConsumerStatefulWidget {
  /// Creates a browser rooted at [workingDirectory].
  const FileBrowser({
    super.key,
    required this.workingDirectory,
    this.service = const FileBrowserService(),
  });

  /// Directory that users cannot navigate above.
  final String workingDirectory;

  /// Filesystem adapter injected into the browser controller.
  final FileBrowserService service;

  @override
  ConsumerState<FileBrowser> createState() => _FileBrowserState();
}

class _FileBrowserState extends ConsumerState<FileBrowser> {
  late NotifierProvider<FileBrowserController, FileBrowserState> _provider;
  var _markdownPreview = false;
  final _rootMenu = MenuController();
  final _rowMenus = <MenuController>[];

  FileBrowserController get _controller => ref.read(_provider.notifier);
  FileBrowserState get _browser => ref.read(_provider);

  @override
  void initState() {
    super.initState();
    _createProvider();
  }

  void _createProvider() {
    final directory = widget.workingDirectory;
    final service = widget.service;
    _provider =
        NotifierProvider.autoDispose<FileBrowserController, FileBrowserState>(
          () => FileBrowserController(
            workingDirectory: directory,
            service: service,
          ),
        );
    _markdownPreview = false;
  }

  @override
  void didUpdateWidget(covariant FileBrowser oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.workingDirectory != widget.workingDirectory ||
        oldWidget.service != widget.service) {
      _createProvider();
    }
  }

  void _dismissMenus() {
    if (_rootMenu.isOpen) _rootMenu.close();
    for (final menu in _rowMenus) {
      if (menu.isOpen) menu.close();
    }
  }

  @override
  Widget build(BuildContext context) {
    final browser = ref.watch(_provider);
    final colors = AtlasColors.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        SizedBox(
          height: 38,
          child: browser.selectedFile == null
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
              '../${_browser.root.path.split(Platform.pathSeparator).last}',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: colors.textSecondary, fontSize: 11.5),
            ),
          ),
        ),
        WorkspaceToolbarButton(
          icon: LucideIcons.refreshCw,
          tooltip: 'Refresh files',
          onPressed: () => unawaited(_controller.refresh()),
        ),
        const SizedBox(width: 6),
      ],
    );
  }

  Widget _buildPreviewToolbar(AtlasColors colors) {
    final selected = _browser.selectedFile;
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
              if (_browser.preview == null) return;
              setState(() => _markdownPreview = !_markdownPreview);
            },
          ),
          const SizedBox(width: 4),
        ],
        WorkspaceToolbarButton(
          icon: LucideIcons.x,
          tooltip: 'Back to files',
          onPressed: _controller.closePreview,
        ),
        const SizedBox(width: 6),
      ],
    );
  }

  Widget _buildContent(AtlasColors colors) {
    final browser = _browser;
    if (browser.selectedFile != null) {
      if (browser.previewError != null) {
        return Padding(
          padding: const EdgeInsets.all(14),
          child: Text(
            browser.previewError!,
            style: TextStyle(color: colors.error, fontSize: 12, height: 1.45),
          ),
        );
      }
      if (_markdownPreview && _isMarkdownFile(browser.selectedFile!)) {
        return SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(12, 6, 12, 18),
          child: SelectionArea(
            child: AtlasMarkdown(
              data: browser.preview ?? '',
              fontFamily: WorkspaceMetrics.monospaceFontFamily,
            ),
          ),
        );
      }
      return SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(12, 6, 12, 18),
        child: SelectableText(
          browser.preview ?? '',
          style: TextStyle(
            color: colors.textPrimary,
            fontFamily: WorkspaceMetrics.monospaceFontFamily,
            fontSize: 14,
            height: 1.45,
          ),
        ),
      );
    }
    if (browser.loading && !browser.loaded) {
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
    if (browser.rootError != null && !browser.loaded) {
      return Padding(
        padding: const EdgeInsets.all(14),
        child: Text(
          browser.rootError!,
          style: TextStyle(color: colors.error, fontSize: 12, height: 1.45),
        ),
      );
    }
    final empty = browser.entries.isEmpty;
    return Stack(
      children: [
        MenuAnchor(
          controller: _rootMenu,
          consumeOutsideTap: true,
          menuChildren: fileBrowserRootMenu(
            colors: colors,
            canPaste: browser.clipboard != null,
            actions: _menuActions(directory: browser.root),
          ),
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
            itemCount: browser.entries.length,
            itemBuilder: (context, index) =>
                _buildRow(browser.entries[index], colors),
          ),
      ],
    );
  }

  Widget _buildRow(FileBrowserEntry node, AtlasColors colors) {
    final isDirectory = node.entity is Directory;
    final actions = _menuActions(
      entity: node.entity,
      directory: isDirectory ? node.entity as Directory : null,
    );
    return FileRowMenu(
      registry: _rowMenus,
      onOpen: _dismissMenus,
      items: isDirectory
          ? fileBrowserFolderMenu(
              colors: colors,
              canPaste: _browser.clipboard != null,
              actions: actions,
            )
          : fileBrowserFileMenu(colors: colors, actions: actions),
      child: WorkspaceHoverSurface(
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: () {
            _dismissMenus();
            if (isDirectory) {
              unawaited(_controller.toggle(node));
            } else {
              setState(() => _markdownPreview = false);
              unawaited(_controller.openFile(node.entity as File));
            }
          },
          child: SizedBox(
            height: 26,
            child: Padding(
              padding: const EdgeInsets.only(left: 6, right: 8),
              child: Row(
                children: [
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

  String _relativePath(String path) {
    final rootSegments = _browser.root.path.split(Platform.pathSeparator);
    final pathSegments = path.split(Platform.pathSeparator);
    var common = 0;
    while (common < rootSegments.length &&
        common < pathSegments.length &&
        rootSegments[common] == pathSegments[common]) {
      common++;
    }
    return [
      ...List.filled(rootSegments.length - common, '..'),
      ...pathSegments.sublist(common),
    ].join(Platform.pathSeparator);
  }

  FileBrowserMenuActions _menuActions({
    FileSystemEntity? entity,
    Directory? directory,
  }) {
    final target = directory ?? _browser.root;
    return FileBrowserMenuActions(
      onNewFile: () => unawaited(_create(target, folder: false)),
      onNewFolder: () => unawaited(_create(target, folder: true)),
      onCopy: () {
        if (entity != null) _controller.copy(entity, cut: false);
      },
      onCut: () {
        if (entity != null) _controller.copy(entity, cut: true);
      },
      onPaste: () => unawaited(_run(() => _controller.pasteInto(target))),
      onCopyPath: () {
        if (entity != null) unawaited(FlutterClipboard.copy(entity.path));
      },
      onCopyRelativePath: () {
        if (entity != null) {
          unawaited(FlutterClipboard.copy(_relativePath(entity.path)));
        }
      },
      onRename: () {
        if (entity != null) unawaited(_rename(entity));
      },
      onReveal: () {
        if (entity != null) {
          unawaited(_run(() => _controller.reveal(entity.path)));
        }
      },
      onTrash: () {
        if (entity != null) unawaited(_trash(entity));
      },
    );
  }

  Future<void> _create(Directory directory, {required bool folder}) async {
    final controller = _controller;
    final name = await promptFileName(
      context,
      title: folder ? 'New Folder' : 'New File',
      hint: 'Name',
      initial: folder ? 'untitled' : 'untitled.md',
    );
    if (name == null || !mounted || !identical(controller, _controller)) return;
    if (!folder) setState(() => _markdownPreview = false);
    await _run(
      () => folder
          ? controller.createFolder(directory, name)
          : controller.createFile(directory, name),
    );
  }

  Future<void> _rename(FileSystemEntity entity) async {
    final controller = _controller;
    final current = entity.path.split(Platform.pathSeparator).last;
    final name = await promptFileName(
      context,
      title: 'Rename',
      hint: 'Name',
      initial: current,
    );
    if (name == null ||
        name == current ||
        !mounted ||
        !identical(controller, _controller)) {
      return;
    }
    await _run(() => controller.rename(entity, name));
  }

  Future<void> _trash(FileSystemEntity entity) async {
    final controller = _controller;
    final name = entity.path.split(Platform.pathSeparator).last;
    final confirmed = await confirmMoveToTrash(context, name);
    if (!confirmed || !mounted || !identical(controller, _controller)) return;
    await _run(() => controller.trash(entity));
  }

  Future<void> _run(Future<void> Function() command) async {
    try {
      await command();
    } on FileSystemException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(error.message)));
    }
  }
}

bool _isMarkdownFile(File file) {
  final extension = file.path.split('.').last.toLowerCase();
  return extension == 'md' || extension == 'markdown';
}

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

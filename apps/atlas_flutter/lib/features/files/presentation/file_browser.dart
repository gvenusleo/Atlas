import 'dart:async';
import 'dart:io';

import 'package:clipboard/clipboard.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/files/application/file_browser_controller.dart';
import 'package:atlas_flutter/features/files/application/file_browser_state.dart';
import 'package:atlas_flutter/features/files/presentation/file_browser_menu.dart';
import 'package:atlas_flutter/l10n/localizations.dart';
import 'package:atlas_flutter/shared/layout/atlas_layout_metrics.dart';
import 'package:atlas_flutter/shared/markdown/atlas_markdown.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_flutter/shared/widgets/window_controls.dart';

/// Renders a lazy file tree and text preview controlled by Riverpod.
class const FileBrowser({
  super.key,

  /// Directory that users cannot navigate above.
  required final String workingDirectory,

  /// Session identity used to keep browser state separate for equal directories.
  final String? sessionKey,
}) extends ConsumerStatefulWidget {
  /// Creates a browser rooted at [workingDirectory].
  this;

  @override
  ConsumerState<FileBrowser> createState() => _FileBrowserState();
}

class _FileBrowserState extends ConsumerState<FileBrowser> {
  FileBrowserProviderKey get _providerKey => (
    sessionKey: widget.sessionKey ?? widget.workingDirectory,
    workingDirectory: widget.workingDirectory,
  );

  NotifierProvider<FileBrowserController, FileBrowserState> get _provider =>
      fileBrowserProvider(_providerKey);
  var _markdownPreview = false;
  final _rootMenu = MenuController();
  final _rowMenus = <MenuController>[];

  FileBrowserController get _controller => ref.read(_provider.notifier);
  FileBrowserState get _browser => ref.read(_provider);

  @override
  void didUpdateWidget(covariant FileBrowser oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.workingDirectory != widget.workingDirectory ||
        oldWidget.sessionKey != widget.sessionKey) {
      _markdownPreview = false;
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
        AtlasToolbarButton(
          icon: LucideIcons.refreshCw,
          tooltip: context.l10n.refreshFiles,
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
          AtlasToolbarButton(
            icon: _markdownPreview
                ? LucideIcons.fileText
                : LucideIcons.bookOpenText,
            tooltip: context.l10n.toggleMarkdownPreview,
            onPressed: () {
              if (_browser.preview == null) return;
              setState(() => _markdownPreview = !_markdownPreview);
            },
          ),
          const SizedBox(width: 4),
        ],
        AtlasToolbarButton(
          icon: LucideIcons.x,
          tooltip: context.l10n.backToFiles,
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
            context.localizeAtlasError(browser.previewError!),
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
              fontFamily: AtlasLayoutMetrics.monospaceFontFamily,
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
            fontFamily: AtlasLayoutMetrics.monospaceFontFamily,
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
          context.localizeAtlasError(browser.rootError!),
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
            context: context,
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
                      context.l10n.emptyFolder,
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
              context: context,
              colors: colors,
              canPaste: _browser.clipboard != null,
              actions: actions,
            )
          : fileBrowserFileMenu(
              context: context,
              colors: colors,
              actions: actions,
            ),
      child: AtlasHoverSurface(
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
                      message: context.localizeAtlasError(node.error!),
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
      title: folder ? context.l10n.newFolder : context.l10n.newFile,
      hint: context.l10n.name,
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
      title: context.l10n.rename,
      hint: context.l10n.name,
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
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(context.localizeAtlasError(error.message))),
      );
    }
  }
}

bool _isMarkdownFile(File file) {
  final extension = file.path.split('.').last.toLowerCase();
  return extension == 'md' || extension == 'markdown';
}

class const _GuideLinePainter({required final Color color})
    extends CustomPainter {
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

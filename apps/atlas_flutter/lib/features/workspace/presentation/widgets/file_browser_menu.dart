import 'dart:io';

import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../../../l10n/localizations.dart';
import '../../../../shared/widgets/animated_caret.dart';
import 'workspace_controls.dart';

/// Actions invoked from a file-browser context menu.
final class const FileBrowserMenuActions({
  /// Creates a file in the current target directory.
  required final VoidCallback onNewFile,

  /// Creates a folder in the current target directory.
  required final VoidCallback onNewFolder,

  /// Copies the current entry.
  required final VoidCallback onCopy,

  /// Cuts the current entry.
  required final VoidCallback onCut,

  /// Pastes into the current target directory.
  required final VoidCallback onPaste,

  /// Copies the absolute path.
  required final VoidCallback onCopyPath,

  /// Copies the path relative to the workspace root.
  required final VoidCallback onCopyRelativePath,

  /// Renames the current entry.
  required final VoidCallback onRename,

  /// Reveals the current entry in the file manager.
  required final VoidCallback onReveal,

  /// Moves the current entry to the trash.
  required final VoidCallback onTrash,
});

/// Builds menu rows for the workspace file browser.
List<Widget> fileBrowserRootMenu({
  required BuildContext context,
  required AtlasColors colors,
  required bool canPaste,
  required FileBrowserMenuActions actions,
}) {
  return [
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.filePlus,
      label: context.l10n.newFile,
      onPressed: actions.onNewFile,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderPlus,
      label: context.l10n.newFolder,
      onPressed: actions.onNewFolder,
    ),
    if (canPaste)
      fileBrowserMenuItem(
        colors,
        icon: LucideIcons.clipboard,
        label: context.l10n.paste,
        onPressed: actions.onPaste,
      ),
  ];
}

/// Builds the context menu for a file row.
List<Widget> fileBrowserFileMenu({
  required BuildContext context,
  required AtlasColors colors,
  required FileBrowserMenuActions actions,
}) {
  return [
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.copy,
      label: context.l10n.copy,
      onPressed: actions.onCopy,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.scissors,
      label: context.l10n.cut,
      onPressed: actions.onCut,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: context.l10n.copyPath,
      onPressed: actions.onCopyPath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: context.l10n.copyRelativePath,
      onPressed: actions.onCopyRelativePath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.pencil,
      label: context.l10n.rename,
      onPressed: actions.onRename,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderOpen,
      label: revealInFileManagerLabel(context),
      onPressed: actions.onReveal,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.trash2,
      label: context.l10n.moveToTrash,
      onPressed: actions.onTrash,
    ),
  ];
}

/// Builds the context menu for a folder row.
List<Widget> fileBrowserFolderMenu({
  required BuildContext context,
  required AtlasColors colors,
  required bool canPaste,
  required FileBrowserMenuActions actions,
}) {
  return [
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.filePlus,
      label: context.l10n.newFile,
      onPressed: actions.onNewFile,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderPlus,
      label: context.l10n.newFolder,
      onPressed: actions.onNewFolder,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.copy,
      label: context.l10n.copy,
      onPressed: actions.onCopy,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.scissors,
      label: context.l10n.cut,
      onPressed: actions.onCut,
    ),
    if (canPaste)
      fileBrowserMenuItem(
        colors,
        icon: LucideIcons.clipboard,
        label: context.l10n.paste,
        onPressed: actions.onPaste,
      ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: context.l10n.copyPath,
      onPressed: actions.onCopyPath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: context.l10n.copyRelativePath,
      onPressed: actions.onCopyRelativePath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.pencil,
      label: context.l10n.rename,
      onPressed: actions.onRename,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderOpen,
      label: revealInFileManagerLabel(context),
      onPressed: actions.onReveal,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.trash2,
      label: context.l10n.moveToTrash,
      onPressed: actions.onTrash,
    ),
  ];
}

/// One styled row in a file-browser context menu.
Widget fileBrowserMenuItem(
  AtlasColors colors, {
  required IconData icon,
  required String label,
  required VoidCallback onPressed,
}) {
  return WorkspaceHoverSurface(
    borderRadius: BorderRadius.circular(AtlasRadii.control),
    child: MenuItemButton(
      style: const ButtonStyle(
        overlayColor: WidgetStatePropertyAll(Colors.transparent),
      ),
      onPressed: onPressed,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 14, color: colors.textSecondary),
          const SizedBox(width: 8),
          Text(
            label,
            style: TextStyle(color: colors.textPrimary, fontSize: 12.5),
          ),
        ],
      ),
    ),
  );
}

/// Platform label for revealing a path in the file manager.
String revealInFileManagerLabel(BuildContext context) {
  if (Platform.isMacOS) {
    return context.l10n.revealInFinder;
  }
  if (Platform.isWindows) {
    return context.l10n.revealInExplorer;
  }
  return context.l10n.revealInFileManager;
}

/// Context menu wrapper that opens on secondary tap.
class const FileRowMenu({
  super.key,

  /// Open row-menu controllers owned by the browser.
  required final List<MenuController> registry,

  /// Closes any already-open menu before this one opens.
  required final VoidCallback onOpen,

  /// Menu rows.
  required final List<Widget> items,

  /// The file-tree row.
  required final Widget child,
}) extends StatefulWidget {
  @override
  State<FileRowMenu> createState() => _FileRowMenuState();
}

class _FileRowMenuState extends State<FileRowMenu> {
  final _controller = MenuController();

  @override
  void initState() {
    super.initState();
    widget.registry.add(_controller);
  }

  @override
  void dispose() {
    widget.registry.remove(_controller);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MenuAnchor(
      controller: _controller,
      consumeOutsideTap: true,
      menuChildren: widget.items,
      child: GestureDetector(
        onSecondaryTapUp: (details) {
          widget.onOpen();
          _controller.open(position: details.localPosition);
        },
        child: widget.child,
      ),
    );
  }
}

/// Prompts for a file or folder name.
Future<String?> promptFileName(
  BuildContext context, {
  required String title,
  required String hint,
  required String initial,
}) async {
  final textController = TextEditingController(text: initial);
  final name = await showDialog<String>(
    context: context,
    builder: (context) {
      final colors = AtlasColors.of(context);
      return AlertDialog(
        title: Text(title),
        content: AnimatedCaret(
          controller: textController,
          child: TextField(
            showCursor: false,
            controller: textController,
            autofocus: true,
            decoration: InputDecoration(hintText: hint),
            onSubmitted: (value) => Navigator.pop(context, value.trim()),
          ),
        ),
        actions: [
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context),
              child: Text(context.l10n.cancel),
            ),
          ),
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () =>
                  Navigator.pop(context, textController.text.trim()),
              child: Text(
                context.l10n.save,
                style: TextStyle(color: colors.accent),
              ),
            ),
          ),
        ],
      );
    },
  );
  WidgetsBinding.instance.addPostFrameCallback((_) {
    textController.dispose();
  });
  if (name == null || name.isEmpty) {
    return null;
  }
  return name;
}

/// Confirms moving [name] to the trash.
Future<bool> confirmMoveToTrash(BuildContext context, String name) async {
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (context) {
      final colors = AtlasColors.of(context);
      return AlertDialog(
        title: Text(context.l10n.moveToTrash),
        content: Text(context.l10n.trashQuestion(name)),
        actions: [
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context, false),
              child: Text(context.l10n.cancel),
            ),
          ),
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context, true),
              child: Text(
                context.l10n.moveToTrash,
                style: TextStyle(color: colors.error),
              ),
            ),
          ),
        ],
      );
    },
  );
  return confirmed == true;
}

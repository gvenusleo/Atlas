import 'dart:io';

import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import 'workspace_controls.dart';

/// Actions invoked from a file-browser context menu.
final class FileBrowserMenuActions {
  /// Creates the action callbacks.
  const FileBrowserMenuActions({
    required this.onNewFile,
    required this.onNewFolder,
    required this.onCopy,
    required this.onCut,
    required this.onPaste,
    required this.onCopyPath,
    required this.onCopyRelativePath,
    required this.onRename,
    required this.onReveal,
    required this.onTrash,
  });

  /// Creates a file in the current target directory.
  final VoidCallback onNewFile;

  /// Creates a folder in the current target directory.
  final VoidCallback onNewFolder;

  /// Copies the current entry.
  final VoidCallback onCopy;

  /// Cuts the current entry.
  final VoidCallback onCut;

  /// Pastes into the current target directory.
  final VoidCallback onPaste;

  /// Copies the absolute path.
  final VoidCallback onCopyPath;

  /// Copies the path relative to the workspace root.
  final VoidCallback onCopyRelativePath;

  /// Renames the current entry.
  final VoidCallback onRename;

  /// Reveals the current entry in the file manager.
  final VoidCallback onReveal;

  /// Moves the current entry to the trash.
  final VoidCallback onTrash;
}

/// Builds menu rows for the workspace file browser.
List<Widget> fileBrowserRootMenu({
  required AtlasColors colors,
  required bool canPaste,
  required FileBrowserMenuActions actions,
}) {
  return [
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.filePlus,
      label: 'New File',
      onPressed: actions.onNewFile,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderPlus,
      label: 'New Folder',
      onPressed: actions.onNewFolder,
    ),
    if (canPaste)
      fileBrowserMenuItem(
        colors,
        icon: LucideIcons.clipboard,
        label: 'Paste',
        onPressed: actions.onPaste,
      ),
  ];
}

/// Builds the context menu for a file row.
List<Widget> fileBrowserFileMenu({
  required AtlasColors colors,
  required FileBrowserMenuActions actions,
}) {
  return [
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.copy,
      label: 'Copy',
      onPressed: actions.onCopy,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.scissors,
      label: 'Cut',
      onPressed: actions.onCut,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: 'Copy Path',
      onPressed: actions.onCopyPath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: 'Copy Relative Path',
      onPressed: actions.onCopyRelativePath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.pencil,
      label: 'Rename',
      onPressed: actions.onRename,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderOpen,
      label: revealInFileManagerLabel,
      onPressed: actions.onReveal,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.trash2,
      label: 'Move to Trash',
      onPressed: actions.onTrash,
    ),
  ];
}

/// Builds the context menu for a folder row.
List<Widget> fileBrowserFolderMenu({
  required AtlasColors colors,
  required bool canPaste,
  required FileBrowserMenuActions actions,
}) {
  return [
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.filePlus,
      label: 'New File',
      onPressed: actions.onNewFile,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderPlus,
      label: 'New Folder',
      onPressed: actions.onNewFolder,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.copy,
      label: 'Copy',
      onPressed: actions.onCopy,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.scissors,
      label: 'Cut',
      onPressed: actions.onCut,
    ),
    if (canPaste)
      fileBrowserMenuItem(
        colors,
        icon: LucideIcons.clipboard,
        label: 'Paste',
        onPressed: actions.onPaste,
      ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: 'Copy Path',
      onPressed: actions.onCopyPath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.clipboardCopy,
      label: 'Copy Relative Path',
      onPressed: actions.onCopyRelativePath,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.pencil,
      label: 'Rename',
      onPressed: actions.onRename,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.folderOpen,
      label: revealInFileManagerLabel,
      onPressed: actions.onReveal,
    ),
    fileBrowserMenuItem(
      colors,
      icon: LucideIcons.trash2,
      label: 'Move to Trash',
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
String get revealInFileManagerLabel {
  if (Platform.isMacOS) {
    return 'Reveal in Finder';
  }
  if (Platform.isWindows) {
    return 'Reveal in Explorer';
  }
  return 'Reveal in File Manager';
}

/// Context menu wrapper that opens on secondary tap.
class FileRowMenu extends StatefulWidget {
  /// Creates a row menu.
  const FileRowMenu({
    super.key,
    required this.registry,
    required this.onOpen,
    required this.items,
    required this.child,
  });

  /// Open row-menu controllers owned by the browser.
  final List<MenuController> registry;

  /// Closes any already-open menu before this one opens.
  final VoidCallback onOpen;

  /// Menu rows.
  final List<Widget> items;

  /// The file-tree row.
  final Widget child;

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
        content: TextField(
          controller: textController,
          autofocus: true,
          decoration: InputDecoration(hintText: hint),
          onSubmitted: (value) => Navigator.pop(context, value.trim()),
        ),
        actions: [
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context),
              child: const Text('Cancel'),
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
              child: Text('Save', style: TextStyle(color: colors.accent)),
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
        title: const Text('Move to Trash'),
        content: Text('Move “$name” to the Trash?'),
        actions: [
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Cancel'),
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
                'Move to Trash',
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

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/files/presentation/file_browser.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';

/// Keeps one [FileBrowser] per session so expand/preview state survives focus changes.
class const FileBrowserHost({
  super.key,

  /// Cache key of the focused session or draft.
  required final String sessionKey,

  /// Working directory of the focused session.
  required final String workingDirectory,
}) extends ConsumerStatefulWidget {
  /// Creates a host for the focused session's file tree.
  this;

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
            sessionKey: key,
          ),
      ],
    );
  }
}

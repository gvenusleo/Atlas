import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/terminal/presentation/terminal_panel.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';

/// Keeps one [TerminalPanel] per session so the shell survives focus changes.
class const TerminalHost({
  super.key,

  /// Cache key of the focused session or draft.
  required final String sessionKey,

  /// Working directory of the focused session.
  required final String workingDirectory,
}) extends ConsumerStatefulWidget {
  /// Creates a host for the focused session's terminal.
  this;

  @override
  ConsumerState<TerminalHost> createState() => _TerminalHostState();
}

class _TerminalHostState extends ConsumerState<TerminalHost> {
  final _directories = <String, String>{};
  final _panelKeys = <String, GlobalKey>{};

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
    _panelKeys.removeWhere((key, _) => !directories.containsKey(key));
    final keys = directories.keys.toList();
    return IndexedStack(
      index: keys.indexOf(widget.sessionKey),
      children: [
        for (final key in keys)
          TerminalPanel(
            key: _panelKeys.putIfAbsent(key, GlobalKey.new),
            workingDirectory: directories[key]!,
          ),
      ],
    );
  }
}

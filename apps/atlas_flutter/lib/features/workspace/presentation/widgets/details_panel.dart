import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../app/runtime_environment.dart';
import '../../application/workspace_controller.dart';
import 'file_browser.dart';
import 'side_panel.dart';
import 'terminal_panel.dart';
import 'workspace_controls.dart';

/// Files and terminal sidebar used by desktop panels and compact drawers.
class DetailsPanel extends ConsumerStatefulWidget {
  /// Creates workspace tools.
  const DetailsPanel({super.key, this.onClose});

  /// Closes the compact drawer when present.
  final VoidCallback? onClose;

  @override
  ConsumerState<DetailsPanel> createState() => _DetailsPanelState();
}

class _DetailsPanelState extends ConsumerState<DetailsPanel> {
  var _terminalCreated = false;

  @override
  Widget build(BuildContext context) {
    final environment = ref.watch(runtimeEnvironmentProvider).environment;
    if (environment == null) {
      return SidePanel(
        semanticLabel: 'Workspace tools',
        compact: widget.onClose != null,
        action: widget.onClose != null
            ? WorkspaceToolbarButton(
                icon: LucideIcons.x,
                tooltip: 'Close workspace tools',
                size: 44,
                onPressed: widget.onClose!,
              )
            : const SizedBox(width: 40),
        child: const PanelEmptyState(
          icon: LucideIcons.triangleAlert,
          message: 'Runtime unavailable',
        ),
      );
    }
    final workingDirectory = ref.watch(
      workspaceProvider.select((s) => s.workingDirectory),
    );
    final sessionKey = ref.watch(workspaceProvider.select((s) => s.activeKey));
    final terminal = ref.watch(workspaceProvider.select((s) => s.showTerminal));
    if (terminal && !_terminalCreated) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && !_terminalCreated) {
          setState(() => _terminalCreated = true);
        }
      });
    }
    return SidePanel(
      semanticLabel: 'Workspace tools',
      compact: widget.onClose != null,
      useCanvasColor: true,
      title: _ToolTabs(terminal: terminal, onChanged: _onToolTabChanged),
      action: widget.onClose != null
          ? WorkspaceToolbarButton(
              icon: LucideIcons.x,
              tooltip: 'Close workspace tools',
              size: 44,
              onPressed: widget.onClose!,
            )
          : const SizedBox(width: 40),
      child: IndexedStack(
        index: terminal ? 1 : 0,
        children: [
          FileBrowserHost(
            sessionKey: sessionKey,
            workingDirectory: workingDirectory,
          ),
          if (_terminalCreated || terminal)
            TerminalHost(
              sessionKey: sessionKey,
              workingDirectory: workingDirectory,
            )
          else
            const SizedBox.shrink(),
        ],
      ),
    );
  }

  void _onToolTabChanged(bool showTerminal) {
    if (showTerminal && !_terminalCreated) {
      setState(() => _terminalCreated = true);
    }
    ref.read(workspaceProvider.notifier).setShowTerminal(showTerminal);
  }
}

class _ToolTabs extends StatelessWidget {
  const _ToolTabs({required this.terminal, required this.onChanged});

  final bool terminal;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        WorkspaceToolbarButton(
          icon: LucideIcons.folder,
          tooltip: 'Files',
          active: !terminal,
          onPressed: () => onChanged(false),
        ),
        const SizedBox(width: 4),
        WorkspaceToolbarButton(
          icon: LucideIcons.terminal,
          tooltip: 'Terminal',
          active: terminal,
          onPressed: () => onChanged(true),
        ),
      ],
    );
  }
}

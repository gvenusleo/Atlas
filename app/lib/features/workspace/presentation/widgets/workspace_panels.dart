import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';
import 'workspace_controls.dart';

/// Sessions sidebar used by both desktop and compact workspace layouts.
class SessionsPanel extends StatelessWidget {
  const SessionsPanel({
    super.key,
    this.onClose,
    this.onToggle,
    this.titlebarInset = false,
  });

  final VoidCallback? onClose;
  final VoidCallback? onToggle;
  final bool titlebarInset;

  @override
  Widget build(BuildContext context) {
    return _SidePanel(
      title: 'Sessions',
      compact: onClose != null,
      leadingInset: titlebarInset ? WorkspaceMetrics.macOSTrafficLightInset : 0,
      action: onClose != null
          ? WorkspaceToolbarButton(
              icon: CupertinoIcons.xmark,
              tooltip: 'Close sessions',
              size: 44,
              onPressed: onClose!,
            )
          : onToggle != null
          ? WorkspaceToolbarButton(
              key: const ValueKey('atlas-left-toggle'),
              icon: CupertinoIcons.sidebar_left,
              tooltip: 'Hide sessions',
              active: true,
              onPressed: onToggle!,
            )
          : null,
      child: const _EmptyPanelState(
        icon: CupertinoIcons.bubble_left_bubble_right,
        message: 'No sessions yet',
      ),
    );
  }
}

/// Details sidebar used by both desktop and compact workspace layouts.
class DetailsPanel extends StatelessWidget {
  const DetailsPanel({super.key, this.onClose, this.onToggle});

  final VoidCallback? onClose;
  final VoidCallback? onToggle;

  @override
  Widget build(BuildContext context) {
    return _SidePanel(
      title: 'Details',
      compact: onClose != null,
      action: onClose != null
          ? WorkspaceToolbarButton(
              icon: CupertinoIcons.xmark,
              tooltip: 'Close details',
              size: 44,
              onPressed: onClose!,
            )
          : onToggle != null
          ? WorkspaceToolbarButton(
              key: const ValueKey('atlas-right-toggle'),
              icon: CupertinoIcons.sidebar_right,
              tooltip: 'Hide details',
              active: true,
              onPressed: onToggle!,
            )
          : null,
      child: const _EmptyPanelState(
        icon: CupertinoIcons.info_circle,
        message: 'No active session',
      ),
    );
  }
}

class _SidePanel extends StatelessWidget {
  const _SidePanel({
    required this.title,
    required this.child,
    this.compact = false,
    this.action,
    this.leadingInset = 0,
  });

  final String title;
  final Widget child;
  final bool compact;
  final Widget? action;
  final double leadingInset;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return ColoredBox(
      color: colors.panel,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          WorkspaceTitlebarDragArea(
            child: SizedBox(
              height: compact
                  ? WorkspaceMetrics.compactToolbarHeight
                  : WorkspaceMetrics.desktopToolbarHeight,
              child: Padding(
                padding: EdgeInsets.only(left: 12 + leadingInset, right: 6),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        title,
                        maxLines: 1,
                        overflow: TextOverflow.clip,
                        style: TextStyle(
                          color: colors.textPrimary,
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ),
                    ?action,
                  ],
                ),
              ),
            ),
          ),
          const Divider(),
          Expanded(child: child),
        ],
      ),
    );
  }
}

/// Central workspace panel and its responsive toolbar.
class WorkspacePanel extends StatelessWidget {
  const WorkspacePanel({
    super.key,
    required this.compact,
    required this.leftActive,
    required this.rightActive,
    required this.onLeftPressed,
    required this.onRightPressed,
  });

  final bool compact;
  final bool leftActive;
  final bool rightActive;
  final VoidCallback onLeftPressed;
  final VoidCallback onRightPressed;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final leftToolbarInset =
        WorkspaceMetrics.usesIntegratedTitlebar && (compact || !leftActive)
        ? WorkspaceMetrics.macOSTrafficLightInset
        : 6.0;

    return ColoredBox(
      key: const ValueKey('atlas-center-panel'),
      color: colors.canvas,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          WorkspaceTitlebarDragArea(
            child: SizedBox(
              height: compact
                  ? WorkspaceMetrics.compactToolbarHeight
                  : WorkspaceMetrics.desktopToolbarHeight,
              child: Padding(
                padding: EdgeInsets.only(left: leftToolbarInset, right: 6),
                child: Row(
                  children: [
                    if (compact || !leftActive)
                      WorkspaceToolbarButton(
                        key: const ValueKey('atlas-left-toggle'),
                        icon: CupertinoIcons.sidebar_left,
                        tooltip: compact ? 'Open sessions' : 'Show sessions',
                        size: compact ? 44 : 40,
                        onPressed: onLeftPressed,
                      ),
                    const SizedBox(width: 4),
                    const WorkspaceTab(label: 'New session'),
                    const Spacer(),
                    if (compact || !rightActive)
                      WorkspaceToolbarButton(
                        key: const ValueKey('atlas-right-toggle'),
                        icon: CupertinoIcons.sidebar_right,
                        tooltip: compact ? 'Open details' : 'Show details',
                        size: compact ? 44 : 40,
                        onPressed: onRightPressed,
                      ),
                  ],
                ),
              ),
            ),
          ),
          const Divider(),
          const Expanded(child: Center(child: _EmptyWorkspaceState())),
        ],
      ),
    );
  }
}

class _EmptyWorkspaceState extends StatelessWidget {
  const _EmptyWorkspaceState();

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(CupertinoIcons.sparkles, color: colors.accent, size: 22),
          const SizedBox(height: 12),
          Text(
            'Start a new session',
            textAlign: TextAlign.center,
            style: TextStyle(
              color: colors.textPrimary,
              fontSize: 14,
              fontWeight: FontWeight.w500,
            ),
          ),
        ],
      ),
    );
  }
}

class _EmptyPanelState extends StatelessWidget {
  const _EmptyPanelState({required this.icon, required this.message});

  final IconData icon;
  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return Center(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, color: colors.textSecondary, size: 18),
            const SizedBox(height: 10),
            Text(
              message,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                height: 1.4,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

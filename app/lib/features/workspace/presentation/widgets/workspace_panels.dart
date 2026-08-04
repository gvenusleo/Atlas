import 'package:flutter/cupertino.dart';
import 'package:flutter/material.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';
import 'workspace_controls.dart';

/// Sessions sidebar used by both desktop and compact workspace layouts.
class SessionsPanel extends StatelessWidget {
  const SessionsPanel({super.key, this.onClose});

  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context) {
    return _SidePanel(
      semanticLabel: 'Sessions',
      compact: onClose != null,
      action: onClose != null
          ? WorkspaceToolbarButton(
              icon: CupertinoIcons.xmark,
              tooltip: 'Close sessions',
              size: 44,
              onPressed: onClose!,
            )
          : const SizedBox(width: 40),
      child: const _EmptyPanelState(
        icon: CupertinoIcons.bubble_left_bubble_right,
        message: 'No sessions yet',
      ),
    );
  }
}

/// Details sidebar used by both desktop and compact workspace layouts.
class DetailsPanel extends StatelessWidget {
  const DetailsPanel({super.key, this.onClose});

  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context) {
    return _SidePanel(
      semanticLabel: 'Details',
      compact: onClose != null,
      action: onClose != null
          ? WorkspaceToolbarButton(
              icon: CupertinoIcons.xmark,
              tooltip: 'Close details',
              size: 44,
              onPressed: onClose!,
            )
          : const SizedBox(width: 40),
      child: const _EmptyPanelState(
        icon: CupertinoIcons.info_circle,
        message: 'No active session',
      ),
    );
  }
}

class _SidePanel extends StatelessWidget {
  const _SidePanel({
    required this.semanticLabel,
    required this.child,
    this.compact = false,
    this.action,
  });

  final String semanticLabel;
  final Widget child;
  final bool compact;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);

    return Semantics(
      container: true,
      label: semanticLabel,
      child: ColoredBox(
        color: colors.panel,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            WorkspaceTitlebarDragArea(
              child: SizedBox(
                height: compact
                    ? WorkspaceMetrics.compactToolbarHeight
                    : WorkspaceMetrics.desktopToolbarHeight,
                child: Align(
                  alignment: Alignment.centerRight,
                  child: Padding(
                    padding: const EdgeInsets.only(right: 6),
                    child: action,
                  ),
                ),
              ),
            ),
            const Divider(),
            Expanded(child: child),
          ],
        ),
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
    required this.onLeftPressed,
    required this.onRightPressed,
  });

  final bool compact;
  final bool leftActive;
  final VoidCallback onLeftPressed;
  final VoidCallback onRightPressed;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final leftToolbarInset =
        WorkspaceMetrics.usesIntegratedTitlebar && (compact || !leftActive)
        ? WorkspaceMetrics.macOSTrafficLightInset
        : 6.0;
    final animationDuration = MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : WorkspaceMetrics.sidebarAnimationDuration;

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
              child: AnimatedPadding(
                duration: animationDuration,
                curve: Curves.easeOutCubic,
                padding: EdgeInsets.only(left: leftToolbarInset, right: 6),
                child: Row(
                  children: [
                    if (compact)
                      WorkspaceToolbarButton(
                        key: const ValueKey('atlas-left-toggle'),
                        icon: CupertinoIcons.sidebar_left,
                        tooltip: 'Open sessions',
                        size: 44,
                        onPressed: onLeftPressed,
                      ),
                    if (!compact)
                      AnimatedContainer(
                        duration: animationDuration,
                        curve: Curves.easeOutCubic,
                        width: leftActive ? 0 : 40,
                      ),
                    const SizedBox(width: 4),
                    const WorkspaceTab(label: 'New session'),
                    const Spacer(),
                    if (compact)
                      WorkspaceToolbarButton(
                        key: const ValueKey('atlas-right-toggle'),
                        icon: CupertinoIcons.sidebar_right,
                        tooltip: 'Open details',
                        size: 44,
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

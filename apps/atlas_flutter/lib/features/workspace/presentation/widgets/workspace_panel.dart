import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../app/runtime_environment.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../workspace_metrics.dart';
import 'conversation_view.dart';
import 'permission_dialog.dart';
import 'workspace_controls.dart';

/// Central conversation panel and its responsive toolbar.
class WorkspacePanel extends ConsumerWidget {
  /// Creates a central workspace.
  const WorkspacePanel({
    super.key,
    required this.compact,
    required this.leftActive,
    required this.onLeftPressed,
    required this.onRightPressed,
    this.startupError,
  });

  /// Runtime startup failure shown in place of the composer.
  final String? startupError;

  /// Whether compact drawer navigation is active.
  final bool compact;

  /// Whether the desktop session sidebar is visible.
  final bool leftActive;

  /// Opens or reveals the session sidebar.
  final VoidCallback onLeftPressed;

  /// Opens or reveals the workspace tools sidebar.
  final VoidCallback onRightPressed;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = AtlasColors.of(context);
    final environment = ref.watch(runtimeEnvironmentProvider).environment;
    final sessionTitle = environment == null
        ? 'New session'
        : ref.watch(workspaceProvider.select((s) => s.sessionTitle));
    final leftToolbarInset =
        WorkspaceMetrics.usesIntegratedTitlebar && (compact || !leftActive)
        ? WorkspaceMetrics.macOSTrafficLightInset
        : 6.0;
    final animationDuration = MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : WorkspaceMetrics.sidebarAnimationDuration;

    return PermissionHost(
      child: ColoredBox(
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
                          icon: LucideIcons.panelLeft,
                          tooltip: 'Open sessions',
                          size: 44,
                          onPressed: onLeftPressed,
                        ),
                      if (!compact)
                        AnimatedContainer(
                          duration: animationDuration,
                          curve: Curves.easeOutCubic,
                          width: leftActive
                              ? 0
                              : WorkspaceMetrics.desktopToolbarButtonSize,
                        ),
                      const SizedBox(width: 4),
                      Expanded(
                        child: Text(
                          sessionTitle,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            color: colors.textPrimary,
                            fontSize: 12,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                      ),
                      if (compact)
                        WorkspaceToolbarButton(
                          key: const ValueKey('atlas-right-toggle'),
                          icon: LucideIcons.panelRight,
                          tooltip: 'Open workspace tools',
                          size: 44,
                          onPressed: onRightPressed,
                        ),
                    ],
                  ),
                ),
              ),
            ),
            const Divider(),
            Expanded(child: _WorkspaceBody(error: startupError)),
          ],
        ),
      ),
    );
  }
}

class _WorkspaceBody extends ConsumerWidget {
  const _WorkspaceBody({this.error});

  final String? error;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final environment = ref.watch(runtimeEnvironmentProvider).environment;
    if (environment == null) {
      return _StartupFailure(
        message: error ?? 'Atlas runtime is not configured.',
      );
    }
    return const SessionPaneHost();
  }
}

class _StartupFailure extends StatelessWidget {
  const _StartupFailure({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(LucideIcons.triangleAlert, color: colors.error, size: 20),
              const SizedBox(height: 12),
              Text(
                message,
                textAlign: TextAlign.center,
                style: TextStyle(
                  color: colors.textSecondary,
                  fontSize: 12.5,
                  height: 1.5,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

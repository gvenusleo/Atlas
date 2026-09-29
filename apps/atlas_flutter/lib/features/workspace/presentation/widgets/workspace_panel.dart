import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/connections/application/runtime_controller.dart';
import 'package:atlas_flutter/features/connections/application/runtime_state.dart';
import 'package:atlas_flutter/features/connections/presentation/remote_connect_view.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/conversation_view.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/permission_dialog.dart';
import 'package:atlas_flutter/l10n/localizations.dart';
import 'package:atlas_flutter/shared/layout/atlas_layout_metrics.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_flutter/shared/widgets/window_controls.dart';

/// Central conversation panel and its responsive toolbar.
class const WorkspacePanel({
  super.key,

  /// Whether compact drawer navigation is active.
  required final bool compact,

  /// Whether the desktop session sidebar is visible.
  required final bool leftActive,

  /// Opens or reveals the session sidebar.
  required final VoidCallback onLeftPressed,

  /// Opens or reveals the workspace tools sidebar.
  required final VoidCallback onRightPressed,

  /// Runtime startup failure shown in place of the composer.
  final String? startupError,
}) extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = AtlasColors.of(context);
    final environment = ref.watch(runtimeEnvironmentProvider).environment;
    final sessionId = ref.watch(workspaceProvider.select((s) => s.sessionId));
    final sessions = ref.watch(workspaceProvider.select((s) => s.sessions));
    final sessionTitle = environment == null || sessionId == null
        ? context.l10n.newSession
        : sessions
                  .where((session) => session.id == sessionId)
                  .map(
                    (session) => session.title.isEmpty
                        ? context.l10n.untitledSession
                        : session.title,
                  )
                  .firstOrNull ??
              context.l10n.session;
    final leftToolbarInset =
        AtlasLayoutMetrics.showsTrafficLights && (compact || !leftActive)
        ? AtlasLayoutMetrics.macOSTrafficLightInset
        : 6.0;
    final animationDuration = MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : AtlasLayoutMetrics.sidebarAnimationDuration;
    final runtimeState = ref.watch(runtimeEnvironmentProvider);
    final remoteStatus = runtimeState.remoteStatus;

    return PermissionHost(
      child: ColoredBox(
        key: const ValueKey('atlas-center-panel'),
        color: colors.canvas,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            AtlasTitlebarDragArea(
              child: SizedBox(
                height: compact
                    ? AtlasLayoutMetrics.compactToolbarHeight
                    : AtlasLayoutMetrics.desktopToolbarHeight,
                child: AnimatedPadding(
                  duration: animationDuration,
                  curve: Curves.easeOutCubic,
                  padding: EdgeInsets.only(left: leftToolbarInset, right: 6),
                  child: Row(
                    children: [
                      if (compact)
                        AtlasToolbarButton(
                          key: const ValueKey('atlas-left-toggle'),
                          icon: LucideIcons.panelLeft,
                          tooltip: context.l10n.openSessions,
                          size: 44,
                          onPressed: onLeftPressed,
                        ),
                      if (!compact)
                        AnimatedContainer(
                          duration: animationDuration,
                          curve: Curves.easeOutCubic,
                          width: leftActive
                              ? 0
                              : AtlasLayoutMetrics.desktopToolbarButtonSize,
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
                      if (remoteStatus == RemoteConnectionStatus.reconnecting)
                        Padding(
                          padding: const EdgeInsets.only(right: 10),
                          child: Row(
                            key: const ValueKey('atlas-remote-status'),
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const SizedBox.square(
                                dimension: 8,
                                child: CircularProgressIndicator(
                                  strokeWidth: 1.5,
                                ),
                              ),
                              const SizedBox(width: 6),
                              Text(
                                context.l10n.reconnecting,
                                style: TextStyle(
                                  color: colors.textSecondary,
                                  fontSize: 11.5,
                                ),
                              ),
                            ],
                          ),
                        ),
                      if (compact)
                        AtlasToolbarButton(
                          key: const ValueKey('atlas-right-toggle'),
                          icon: LucideIcons.panelRight,
                          tooltip: context.l10n.openWorkspaceTools,
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

class const _WorkspaceBody({final String? error}) extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final environment = ref.watch(runtimeEnvironmentProvider).environment;
    if (environment == null) {
      // Mobile clients start without a runtime and manage remote connections
      // here; desktop configuration failures keep the startup message.
      if (error == null) {
        return const RemoteConnectView();
      }
      return _StartupFailure(message: context.localizeAtlasError(error!));
    }
    return const SessionPaneHost();
  }
}

class const _StartupFailure({required final String message})
    extends StatelessWidget {
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

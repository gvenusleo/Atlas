import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../app/runtime_environment.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';
import '../workspace_metrics.dart';
import 'conversation_input.dart';
import 'conversation_view.dart';
import 'file_browser.dart';
import 'terminal_panel.dart';
import 'workspace_controls.dart';

/// Sessions sidebar used by desktop panels and compact drawers.
class SessionsPanel extends ConsumerWidget {
  /// Creates a session list.
  const SessionsPanel({super.key, this.onClose});

  /// Closes the compact drawer when present.
  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final environment = ref.watch(runtimeEnvironmentProvider);
    return _SidePanel(
      semanticLabel: 'Sessions',
      compact: onClose != null,
      action: onClose != null
          ? WorkspaceToolbarButton(
              icon: LucideIcons.x,
              tooltip: 'Close sessions',
              size: 44,
              onPressed: onClose!,
            )
          : const SizedBox(width: 40),
      child: environment == null
          ? const _PanelEmptyState(
              icon: LucideIcons.triangleAlert,
              message: 'Runtime unavailable',
            )
          : _SessionList(onClose: onClose),
    );
  }
}

class _SessionList extends ConsumerWidget {
  const _SessionList({this.onClose});

  final VoidCallback? onClose;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final colors = AtlasColors.of(context);
    final sessions = ref.watch(workspaceProvider.select((s) => s.sessions));
    final busy = ref.watch(workspaceProvider.select((s) => s.busy));
    final loading = ref.watch(
      workspaceProvider.select((s) => s.loadingSessions),
    );
    final sessionId = ref.watch(workspaceProvider.select((s) => s.sessionId));
    final controller = ref.read(workspaceProvider.notifier);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(10, 6, 30, 6),
          child: InkWell(
            key: const ValueKey('atlas-search-session'),
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            onTap: null,
            child: Container(
              height: 36,
              padding: const EdgeInsets.symmetric(horizontal: 10),
              decoration: BoxDecoration(
                color: colors.raised,
                borderRadius: BorderRadius.circular(AtlasRadii.control),
              ),
              child: Row(
                children: [
                  Icon(
                    LucideIcons.search,
                    color: colors.textSecondary,
                    size: 15,
                  ),
                  const SizedBox(width: 8),
                  Text(
                    'Search session',
                    style: TextStyle(
                      color: colors.textPrimary,
                      fontSize: 12.5,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
        Expanded(
          child: loading && sessions.isEmpty
              ? Center(
                  child: SizedBox.square(
                    dimension: 18,
                    child: CircularProgressIndicator(
                      strokeWidth: 1.5,
                      color: colors.accent,
                    ),
                  ),
                )
              : sessions.isEmpty
              ? const _PanelEmptyState(
                  icon: LucideIcons.messageCircle,
                  message: 'No sessions yet',
                )
              : RefreshIndicator(
                  onRefresh: controller.refreshSessions,
                  child: ListView.builder(
                    padding: const EdgeInsets.fromLTRB(6, 2, 6, 12),
                    itemCount: sessions.length,
                    itemBuilder: (context, index) {
                      final session = sessions[index];
                      final selected = session.id == sessionId;
                      return Padding(
                        padding: const EdgeInsets.symmetric(vertical: 1),
                        child: InkWell(
                          borderRadius: BorderRadius.circular(
                            AtlasRadii.control,
                          ),
                          onTap: busy
                              ? null
                              : () async {
                                  await controller.resume(session.id);
                                  onClose?.call();
                                },
                          child: Container(
                            constraints: const BoxConstraints(minHeight: 50),
                            padding: const EdgeInsets.symmetric(
                              horizontal: 9,
                              vertical: 7,
                            ),
                            decoration: BoxDecoration(
                              color: selected ? colors.raised : null,
                              borderRadius: BorderRadius.circular(
                                AtlasRadii.control,
                              ),
                            ),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              mainAxisAlignment: MainAxisAlignment.center,
                              children: [
                                Text(
                                  session.title.isEmpty
                                      ? 'Untitled session'
                                      : session.title,
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: TextStyle(
                                    color: colors.textPrimary,
                                    fontSize: 12.5,
                                    fontWeight: selected
                                        ? FontWeight.w600
                                        : FontWeight.w400,
                                  ),
                                ),
                                const SizedBox(height: 3),
                                Text(
                                  _relativeTime(session.updatedAt),
                                  style: TextStyle(
                                    color: colors.textSecondary,
                                    fontSize: 10.5,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ),
                      );
                    },
                  ),
                ),
        ),
      ],
    );
  }

  static String _relativeTime(DateTime value) {
    final elapsed = DateTime.now().toUtc().difference(value.toUtc());
    if (elapsed.inMinutes < 1) {
      return 'Now';
    }
    if (elapsed.inHours < 1) {
      return '${elapsed.inMinutes}m';
    }
    if (elapsed.inDays < 1) {
      return '${elapsed.inHours}h';
    }
    if (elapsed.inDays < 7) {
      return '${elapsed.inDays}d';
    }
    return '${value.toLocal().month}/${value.toLocal().day}';
  }
}

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
  var _terminal = false;
  var _terminalCreated = false;

  @override
  Widget build(BuildContext context) {
    final environment = ref.watch(runtimeEnvironmentProvider);
    if (environment == null) {
      return _SidePanel(
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
        child: const _PanelEmptyState(
          icon: LucideIcons.triangleAlert,
          message: 'Runtime unavailable',
        ),
      );
    }
    final workingDirectory = ref.watch(
      workspaceProvider.select((s) => s.workingDirectory),
    );
    return _SidePanel(
      semanticLabel: 'Workspace tools',
      compact: widget.onClose != null,
      title: _ToolTabs(
        terminal: _terminal,
        onChanged: (terminal) => setState(() {
          _terminal = terminal;
          if (terminal) {
            _terminalCreated = true;
          }
        }),
      ),
      action: widget.onClose != null
          ? WorkspaceToolbarButton(
              icon: LucideIcons.x,
              tooltip: 'Close workspace tools',
              size: 44,
              onPressed: widget.onClose!,
            )
          : const SizedBox(width: 40),
      child: IndexedStack(
        index: _terminal ? 1 : 0,
        children: [
          FileBrowser(workingDirectory: workingDirectory),
          if (_terminalCreated)
            TerminalPanel(workingDirectory: workingDirectory)
          else
            const SizedBox.shrink(),
        ],
      ),
    );
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

class _SidePanel extends StatelessWidget {
  const _SidePanel({
    required this.semanticLabel,
    required this.child,
    this.compact = false,
    this.title,
    this.action,
  });

  final String semanticLabel;
  final Widget child;
  final bool compact;
  final Widget? title;
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
                child: Row(
                  children: [
                    Expanded(
                      child: Padding(
                        padding: const EdgeInsets.only(left: 4, top: 6),
                        child: Align(
                          alignment: Alignment.topLeft,
                          child: title,
                        ),
                      ),
                    ),
                    Padding(
                      padding: const EdgeInsets.only(right: 6),
                      child: action,
                    ),
                  ],
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
    final environment = ref.watch(runtimeEnvironmentProvider);
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
    );
  }
}

class _WorkspaceBody extends ConsumerWidget {
  const _WorkspaceBody({this.error});

  final String? error;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final environment = ref.watch(runtimeEnvironmentProvider);
    if (environment == null) {
      return _StartupFailure(
        message: error ?? 'Atlas runtime is not configured.',
      );
    }
    return const Column(
      children: [
        Expanded(child: ConversationView()),
        Align(alignment: Alignment.center, child: ConversationInput()),
      ],
    );
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
              Icon(
                LucideIcons.triangleAlert,
                color: colors.error,
                size: 20,
              ),
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

class _PanelEmptyState extends StatelessWidget {
  const _PanelEmptyState({required this.icon, required this.message});

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

import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:file_selector/file_selector.dart';
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

/// Picks a working directory for a new session; overridable in tests.
final directoryPickerProvider = Provider<Future<String?> Function()>(
  (ref) => getDirectoryPath,
);

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

/// A time-bucketed group of sessions ordered newest first.
final class SessionGroup {
  /// Creates a session group.
  const SessionGroup({required this.label, required this.sessions});

  /// The relative time label shared by the group.
  final String label;

  /// Sessions in descending update order.
  final List<SessionSummary> sessions;
}

/// Groups sessions by recency, newest first, in fixed time buckets.
List<SessionGroup> groupSessionsByTime(List<SessionSummary> sessions) {
  final now = DateTime.now();
  final today = DateTime(now.year, now.month, now.day);
  final yesterday = today.subtract(const Duration(days: 1));
  final weekStart = today.subtract(const Duration(days: 7));
  final monthStart = today.subtract(const Duration(days: 30));

  final byLabel = <String, List<SessionSummary>>{};
  for (final session in sessions) {
    final updated = session.updatedAt.toLocal();
    final day = DateTime(updated.year, updated.month, updated.day);
    final String label;
    if (!day.isBefore(today)) {
      label = 'Today';
    } else if (!day.isBefore(yesterday)) {
      label = 'Yesterday';
    } else if (!day.isBefore(weekStart)) {
      label = 'This Week';
    } else if (!day.isBefore(monthStart)) {
      label = 'This Month';
    } else {
      label = 'Earlier';
    }
    byLabel.putIfAbsent(label, () => []).add(session);
  }
  return [
    for (final label in const [
      'Today',
      'Yesterday',
      'This Week',
      'This Month',
      'Earlier',
    ])
      if (byLabel[label] case final sessions?)
        SessionGroup(
          label: label,
          sessions: [...sessions]
            ..sort((a, b) => b.updatedAt.compareTo(a.updatedAt)),
        ),
  ];
}

class _SessionList extends ConsumerStatefulWidget {
  const _SessionList({this.onClose});

  final VoidCallback? onClose;

  @override
  ConsumerState<_SessionList> createState() => _SessionListState();
}

class _SessionListState extends ConsumerState<_SessionList> {
  /// Directories whose session groups are collapsed.
  final Set<String> _collapsed = {};

  /// Starts a fresh session in the current working directory.
  void _newSessionHere() {
    final controller = ref.read(workspaceProvider.notifier);
    controller.newSession();
    unawaited(controller.refreshSessions());
  }

  /// Picks a working directory and starts a fresh session in it.
  Future<void> _newSessionInFolder() async {
    final picker = ref.read(directoryPickerProvider);
    final directory = await picker();
    if (directory == null || !mounted) {
      return;
    }
    ref.read(workspaceWorkingDirectoryProvider.notifier).set(directory);
    final controller = ref.read(workspaceProvider.notifier);
    controller.newSession();
    unawaited(controller.refreshSessions());
  }

  void _handleNewSessionAction(_NewSessionAction action) {
    switch (action) {
      case _NewSessionAction.here:
        _newSessionHere();
      case _NewSessionAction.folder:
        unawaited(_newSessionInFolder());
    }
  }

  @override
  Widget build(BuildContext context) {
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
          padding: const EdgeInsets.fromLTRB(8, 8, 4, 4),
          child: Column(
            children: [
              PopupMenuButton<_NewSessionAction>(
                key: const ValueKey('atlas-new-session-button'),
                position: PopupMenuPosition.under,
                offset: const Offset(0, 4),
                constraints: const BoxConstraints(minWidth: 112, maxWidth: 360),
                onSelected: _handleNewSessionAction,
                itemBuilder: (context) => const [
                  PopupMenuItem(
                    value: _NewSessionAction.here,
                    height: 32,
                    child: _NewSessionMenuItem(
                      icon: LucideIcons.plus,
                      label: 'New session here',
                    ),
                  ),
                  PopupMenuItem(
                    value: _NewSessionAction.folder,
                    height: 32,
                    child: _NewSessionMenuItem(
                      icon: LucideIcons.folderOpen,
                      label: 'New session in folder...',
                    ),
                  ),
                ],
                child: const _SidebarActionButton(
                  key: ValueKey('atlas-new-task-button'),
                  icon: LucideIcons.pencil,
                  label: 'New Task',
                ),
              ),
              const SizedBox(height: 2),
              const _SidebarActionButton(
                key: ValueKey('atlas-search-session'),
                icon: LucideIcons.search,
                label: 'Search',
              ),
            ],
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
                  child: ListView(
                    padding: const EdgeInsets.fromLTRB(8, 4, 4, 12),
                    children: [
                      for (final group in groupSessionsByTime(sessions)) ...[
                        _SessionGroupHeader(
                          label: group.label,
                          collapsed: _collapsed.contains(group.label),
                          onToggle: () => setState(() {
                            if (!_collapsed.add(group.label)) {
                              _collapsed.remove(group.label);
                            }
                          }),
                        ),
                        if (!_collapsed.contains(group.label))
                          for (final session in group.sessions)
                            _SessionTile(
                              session: session,
                              selected: session.id == sessionId,
                              busy: busy,
                              onTap: () async {
                                await controller.resume(session.id);
                                widget.onClose?.call();
                              },
                            ),
                      ],
                    ],
                  ),
                ),
        ),
      ],
    );
  }
}

/// Directory header above a group of sessions.
class _SessionGroupHeader extends StatelessWidget {
  const _SessionGroupHeader({
    required this.label,
    required this.collapsed,
    required this.onToggle,
  });

  final String label;
  final bool collapsed;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return InkWell(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      onTap: onToggle,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(8, 14, 4, 6),
        child: Row(
          children: [
            Text(
              label,
              key: ValueKey('session-group-$label'),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
            if (collapsed) ...[
              const SizedBox(width: 4),
              Icon(
                LucideIcons.chevronRight,
                size: 12,
                color: colors.textSecondary,
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// One selectable session row inside a directory group.
class _SessionTile extends StatelessWidget {
  const _SessionTile({
    required this.session,
    required this.selected,
    required this.busy,
    required this.onTap,
  });

  final SessionSummary session;
  final bool selected;
  final bool busy;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 0.5),
      child: InkWell(
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        onTap: busy ? null : onTap,
        child: Container(
          constraints: const BoxConstraints(minHeight: 52),
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: BoxDecoration(
            color: selected ? colors.raised : null,
            borderRadius: BorderRadius.circular(AtlasRadii.control),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      session.title.isEmpty
                          ? 'Untitled session'
                          : session.title,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        color: colors.textPrimary,
                        fontSize: 13,
                        fontWeight: selected
                            ? FontWeight.w600
                            : FontWeight.w500,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Row(
                      children: [
                        Icon(
                          LucideIcons.folder,
                          size: 12,
                          color: colors.textSecondary,
                        ),
                        const SizedBox(width: 4),
                        Flexible(
                          child: Text(
                            WorkspaceMetrics.directoryLabel(
                              session.workingDirectory,
                            ),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              color: colors.textSecondary,
                              fontSize: 12,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Padding(
                padding: const EdgeInsets.only(top: 21),
                child: Text(
                  _relativeTime(session.updatedAt),
                  style: TextStyle(color: colors.textSecondary, fontSize: 12),
                ),
              ),
            ],
          ),
        ),
      ),
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

/// Actions offered by the new-session menu.
enum _NewSessionAction { here, folder }

/// One row of the new-session menu.
class _NewSessionMenuItem extends StatelessWidget {
  const _NewSessionMenuItem({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Row(
      children: [
        Icon(icon, size: 14, color: colors.textSecondary),
        const SizedBox(width: 8),
        Text(
          label,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(color: colors.textPrimary, fontSize: 12.5),
        ),
      ],
    );
  }
}

/// Sidebar action row with an icon and label, hover-highlighted.
class _SidebarActionButton extends StatefulWidget {
  const _SidebarActionButton({
    super.key,
    required this.icon,
    required this.label,
  });

  final IconData icon;
  final String label;

  @override
  State<_SidebarActionButton> createState() => _SidebarActionButtonState();
}

class _SidebarActionButtonState extends State<_SidebarActionButton> {
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return MouseRegion(
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() => _hovered = false),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 120),
        height: 30,
        padding: const EdgeInsets.symmetric(horizontal: 10),
        decoration: BoxDecoration(
          color: _hovered ? colors.raised : null,
          borderRadius: BorderRadius.circular(8),
        ),
        child: Row(
          children: [
            Icon(widget.icon, size: 14, color: colors.textSecondary),
            const SizedBox(width: 6),
            Text(
              widget.label,
              style: TextStyle(color: colors.textSecondary, fontSize: 12.5),
            ),
          ],
        ),
      ),
    );
  }
}

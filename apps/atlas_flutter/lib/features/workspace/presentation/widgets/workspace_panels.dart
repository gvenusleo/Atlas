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

  /// Opens the new-session menu anchored to the sidebar action.
  final _newSessionMenu = MenuController();

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
    final controller = ref.read(workspaceProvider.notifier);
    controller.newSession(workingDirectory: directory);
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

  /// Renames a session via a dialog.
  Future<void> _renameSession(SessionSummary session) async {
    final colors = AtlasColors.of(context);
    final textController = TextEditingController(text: session.title);
    final title = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Rename session'),
        content: TextField(
          controller: textController,
          autofocus: true,
          decoration: const InputDecoration(hintText: 'Title'),
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
      ),
    );
    textController.dispose();
    if (title == null || title.isEmpty || title == session.title) {
      return;
    }
    await ref.read(workspaceProvider.notifier).renameSession(session.id, title);
  }

  /// Deletes a session after confirmation.
  Future<void> _deleteSession(SessionSummary session) async {
    final colors = AtlasColors.of(context);
    final label = session.title.isEmpty ? 'Untitled session' : session.title;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete session'),
        content: Text('Delete "$label"?'),
        actions: [
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context, false),
              child: Text(
                'Cancel',
                style: TextStyle(color: colors.textPrimary),
              ),
            ),
          ),
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: TextButton(
              style: const ButtonStyle(
                overlayColor: WidgetStatePropertyAll(Colors.transparent),
              ),
              onPressed: () => Navigator.pop(context, true),
              child: Text('Delete', style: TextStyle(color: colors.error)),
            ),
          ),
        ],
      ),
    );
    if (confirmed != true) {
      return;
    }
    await ref.read(workspaceProvider.notifier).deleteSession(session.id);
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final sessions = ref.watch(workspaceProvider.select((s) => s.sessions));
    final runningIds = ref.watch(
      workspaceProvider.select((s) => s.runningSessionIds),
    );
    final completedIds = ref.watch(
      workspaceProvider.select((s) => s.completedSessionIds),
    );
    final loading = ref.watch(
      workspaceProvider.select((s) => s.loadingSessions),
    );
    final sessionId = ref.watch(workspaceProvider.select((s) => s.sessionId));
    final controller = ref.read(workspaceProvider.notifier);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(8, 8, 4, 8),
          child: Column(
            children: [
              MenuAnchor(
                key: const ValueKey('atlas-new-session-button'),
                controller: _newSessionMenu,
                style: MenuStyle(alignment: AlignmentDirectional.bottomStart),
                menuChildren: [
                  WorkspaceHoverSurface(
                    borderRadius: BorderRadius.circular(AtlasRadii.control),
                    child: MenuItemButton(
                      style: const ButtonStyle(
                        overlayColor: WidgetStatePropertyAll(
                          Colors.transparent,
                        ),
                      ),
                      onPressed: () {
                        _newSessionMenu.close();
                        _handleNewSessionAction(_NewSessionAction.here);
                      },
                      child: const _NewSessionMenuItem(
                        icon: LucideIcons.plus,
                        label: 'New session here',
                      ),
                    ),
                  ),
                  WorkspaceHoverSurface(
                    borderRadius: BorderRadius.circular(AtlasRadii.control),
                    child: MenuItemButton(
                      style: const ButtonStyle(
                        overlayColor: WidgetStatePropertyAll(
                          Colors.transparent,
                        ),
                      ),
                      onPressed: () {
                        _newSessionMenu.close();
                        _handleNewSessionAction(_NewSessionAction.folder);
                      },
                      child: const _NewSessionMenuItem(
                        icon: LucideIcons.folderOpen,
                        label: 'New session in folder...',
                      ),
                    ),
                  ),
                ],
                child: _SidebarActionButton(
                  icon: LucideIcons.pencil,
                  label: 'New Session',
                  onTap: () => _newSessionMenu.open(),
                ),
              ),
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
                    padding: const EdgeInsets.fromLTRB(8, 0, 4, 12),
                    children: [
                      for (final group in groupSessionsByTime(sessions)) ...[
                        const SizedBox(height: 8),
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
                              running: runningIds.contains(session.id),
                              completed:
                                  session.id != sessionId &&
                                  completedIds.contains(session.id),
                              onTap: () async {
                                await controller.resume(session.id);
                                widget.onClose?.call();
                              },
                              onRename: () =>
                                  unawaited(_renameSession(session)),
                              onDelete: () =>
                                  unawaited(_deleteSession(session)),
                            ),
                      ],
                    ],
                  ),
                ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(8, 8, 4, 8),
          child: Align(
            alignment: Alignment.bottomLeft,
            child: WorkspaceToolbarButton(
              icon: LucideIcons.settings,
              tooltip: 'Settings',
              onPressed: () {},
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
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onToggle,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(6, 4, 6, 4),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.center,
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
                const SizedBox(width: 2),
                Icon(
                  LucideIcons.chevronRight,
                  size: 12,
                  color: colors.textSecondary,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

/// One selectable session row inside a directory group.
class _SessionTile extends StatefulWidget {
  const _SessionTile({
    required this.session,
    required this.selected,
    required this.running,
    required this.completed,
    required this.onTap,
    required this.onRename,
    required this.onDelete,
  });

  final SessionSummary session;
  final bool selected;
  final bool running;
  final bool completed;
  final VoidCallback onTap;
  final VoidCallback onRename;
  final VoidCallback onDelete;

  @override
  State<_SessionTile> createState() => _SessionTileState();
}

class _SessionTileState extends State<_SessionTile> {
  final _menuController = MenuController();

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final session = widget.session;
    return MenuAnchor(
      controller: _menuController,
      menuChildren: [
        WorkspaceHoverSurface(
          borderRadius: BorderRadius.circular(AtlasRadii.control),
          child: MenuItemButton(
            style: const ButtonStyle(
              overlayColor: WidgetStatePropertyAll(Colors.transparent),
            ),
            onPressed: () {
              _menuController.close();
              widget.onRename();
            },
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(LucideIcons.pencil, size: 14, color: colors.textSecondary),
                const SizedBox(width: 8),
                Text(
                  'Rename',
                  style: TextStyle(color: colors.textPrimary, fontSize: 12.5),
                ),
              ],
            ),
          ),
        ),
        WorkspaceHoverSurface(
          borderRadius: BorderRadius.circular(AtlasRadii.control),
          child: MenuItemButton(
            style: const ButtonStyle(
              overlayColor: WidgetStatePropertyAll(Colors.transparent),
            ),
            onPressed: () {
              _menuController.close();
              widget.onDelete();
            },
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(LucideIcons.trash2, size: 14, color: colors.textSecondary),
                const SizedBox(width: 8),
                Text(
                  'Delete',
                  style: TextStyle(color: colors.textPrimary, fontSize: 12.5),
                ),
              ],
            ),
          ),
        ),
      ],
      child: WorkspaceHoverSurface(
        // Selected rows keep the highlight while not hovered.
        color: widget.selected ? colors.raised : null,
        borderRadius: BorderRadius.circular(AtlasRadii.control),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: widget.onTap,
          onSecondaryTapUp: (details) {
            _menuController.open(position: details.localPosition);
          },
          child: Container(
            constraints: const BoxConstraints(minHeight: 52),
            padding: const EdgeInsets.all(8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        session.title.isEmpty
                            ? 'Untitled session'
                            : session.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          color: colors.textPrimary,
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                    ),
                    if (widget.running || widget.completed) ...[
                      const SizedBox(width: 6),
                      _SessionStatusMark(
                        sessionId: session.id,
                        running: widget.running,
                      ),
                    ],
                  ],
                ),
                const SizedBox(height: 2),
                Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    Icon(
                      LucideIcons.folder,
                      size: 12,
                      color: colors.textSecondary,
                    ),
                    const SizedBox(width: 4),
                    Flexible(
                      fit: FlexFit.tight,
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
                    Text(
                      _relativeTime(session.updatedAt),
                      style: TextStyle(
                        color: colors.textSecondary,
                        fontSize: 12,
                      ),
                    ),
                  ],
                ),
              ],
            ),
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

/// Quiet status mark to the right of a session title.
class _SessionStatusMark extends StatelessWidget {
  const _SessionStatusMark({required this.sessionId, required this.running});

  final SessionId sessionId;
  final bool running;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return SizedBox.square(
      dimension: 10,
      child: running
          ? CircularProgressIndicator(
              key: ValueKey('session-running-${sessionId.value}'),
              strokeWidth: 1.5,
              color: colors.textSecondary,
            )
          : Center(
              child: Container(
                key: ValueKey('session-completed-${sessionId.value}'),
                width: 6,
                height: 6,
                decoration: BoxDecoration(
                  color: colors.accent,
                  shape: BoxShape.circle,
                ),
              ),
            ),
    );
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
    final sessionKey = ref.watch(workspaceProvider.select((s) => s.activeKey));
    final terminal = ref.watch(workspaceProvider.select((s) => s.showTerminal));
    if (terminal && !_terminalCreated) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && !_terminalCreated) {
          setState(() => _terminalCreated = true);
        }
      });
    }
    return _SidePanel(
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

class _SidePanel extends StatelessWidget {
  const _SidePanel({
    required this.semanticLabel,
    required this.child,
    this.compact = false,
    this.title,
    this.action,
    this.useCanvasColor = false,
  });

  final String semanticLabel;
  final Widget child;
  final bool compact;
  final Widget? title;
  final Widget? action;
  final bool useCanvasColor;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Semantics(
      container: true,
      label: semanticLabel,
      child: ColoredBox(
        color: useCanvasColor ? colors.canvas : colors.panel,
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
class _SidebarActionButton extends StatelessWidget {
  const _SidebarActionButton({
    super.key,
    required this.icon,
    required this.label,
    this.onTap,
  });

  final IconData icon;
  final String label;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
          child: Row(
            children: [
              Icon(icon, size: 14, color: colors.textPrimary),
              const SizedBox(width: 12),
              Text(
                label,
                style: TextStyle(color: colors.textPrimary, fontSize: 13),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

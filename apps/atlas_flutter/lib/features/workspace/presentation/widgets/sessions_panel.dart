import 'dart:async';

import 'package:flutter/services.dart';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../remote_connection/application/runtime_controller.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../../../shared/widgets/animated_caret.dart';
import '../../application/workspace_controller.dart';
import '../workspace_metrics.dart';
import 'side_panel.dart';
import 'workspace_controls.dart';

/// Picks a working directory for a new session; overridable in tests.
final directoryPickerProvider = Provider<Future<String?> Function()>(
  (ref) => FilePicker.getDirectoryPath,
);

/// Sessions sidebar used by desktop panels and compact drawers.
class const SessionsPanel({
  super.key,

  /// Closes the compact drawer when present.
  final VoidCallback? onClose,
}) extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final environment = ref.watch(runtimeEnvironmentProvider).environment;
    return SidePanel(
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
      footer: _SessionsPanelToolbar(compact: onClose != null),
      child: environment == null
          ? const PanelEmptyState(
              icon: LucideIcons.triangleAlert,
              message: 'Runtime unavailable',
            )
          : _SessionList(onClose: onClose),
    );
  }
}

/// Bottom toolbar of the sessions panel holding the settings entry.
class const _SessionsPanelToolbar({required final bool compact})
    extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 8, 4, 8),
      child: Align(
        alignment: Alignment.bottomLeft,
        child: WorkspaceToolbarButton(
          icon: LucideIcons.settings,
          tooltip: 'Settings',
          size: compact ? 44 : null,
          onPressed: () => context.push('/settings'),
        ),
      ),
    );
  }
}

/// A time-bucketed group of sessions ordered newest first.
final class const SessionGroup({
  /// The relative time label shared by the group.
  required final String label,

  /// Sessions in descending update order.
  required final List<SessionSummary> sessions,
});

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

class const _SessionList({final VoidCallback? onClose})
    extends ConsumerStatefulWidget {
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
        content: AnimatedCaret(
          controller: textController,
          child: TextField(
            showCursor: false,
            controller: textController,
            autofocus: true,
            decoration: const InputDecoration(hintText: 'Title'),
            onSubmitted: (value) => Navigator.pop(context, value.trim()),
          ),
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
    final rows = [
      for (final group in groupSessionsByTime(sessions)) ...[
        (group: group.label, session: null),
        if (!_collapsed.contains(group.label))
          for (final session in group.sessions)
            (group: group.label, session: session),
      ],
    ];
    final keys = [
      for (final row in rows)
        ValueKey(
          row.session == null
              ? 'group-${row.group}'
              : 'session-${row.session!.id.value}',
        ),
    ];
    final indexByKey = {for (var i = 0; i < keys.length; i++) keys[i]: i};
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
              ? const PanelEmptyState(
                  icon: LucideIcons.messageCircle,
                  message: 'No sessions yet',
                )
              : RefreshIndicator(
                  onRefresh: controller.refreshSessions,
                  child: ListView.builder(
                    padding: const EdgeInsets.fromLTRB(8, 0, 4, 12),
                    itemCount: rows.length,
                    findChildIndexCallback: (key) => indexByKey[key],
                    itemBuilder: (context, index) {
                      final row = rows[index];
                      final session = row.session;
                      if (session == null) {
                        return Padding(
                          key: keys[index],
                          padding: const EdgeInsets.only(top: 8),
                          child: _SessionGroupHeader(
                            label: row.group,
                            collapsed: _collapsed.contains(row.group),
                            onToggle: () => setState(() {
                              if (!_collapsed.add(row.group)) {
                                _collapsed.remove(row.group);
                              }
                            }),
                          ),
                        );
                      }
                      return _SessionTile(
                        key: keys[index],
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
                        onRename: () => unawaited(_renameSession(session)),
                        onDelete: () => unawaited(_deleteSession(session)),
                      );
                    },
                  ),
                ),
        ),
      ],
    );
  }
}

/// Directory header above a group of sessions.
class const _SessionGroupHeader({
  required final String label,
  required final bool collapsed,
  required final VoidCallback onToggle,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: Material(
        type: MaterialType.transparency,
        child: InkWell(
          focusColor: colors.raised,
          borderRadius: BorderRadius.circular(AtlasRadii.control),
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
      ),
    );
  }
}

/// One selectable session row inside a directory group.
class const _SessionTile({
  super.key,
  required final SessionSummary session,
  required final bool selected,
  required final bool running,
  required final bool completed,
  required final VoidCallback onTap,
  required final VoidCallback onRename,
  required final VoidCallback onDelete,
}) extends StatefulWidget {
  @override
  State<_SessionTile> createState() => _SessionTileState();
}

class _SessionTileState extends State<_SessionTile> {
  final _menuController = MenuController();
  final _focusNode = FocusNode();

  @override
  void dispose() {
    _focusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final session = widget.session;
    return CallbackShortcuts(
      bindings: {
        const SingleActivator(LogicalKeyboardKey.f10, shift: true): () =>
            _menuController.open(),
      },
      child: MenuAnchor(
        controller: _menuController,
        childFocusNode: _focusNode,
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
                  Icon(
                    LucideIcons.pencil,
                    size: 14,
                    color: colors.textSecondary,
                  ),
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
                  Icon(
                    LucideIcons.trash2,
                    size: 14,
                    color: colors.textSecondary,
                  ),
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
          child: Material(
            type: MaterialType.transparency,
            child: InkWell(
              focusColor: colors.raised,
              borderRadius: BorderRadius.circular(AtlasRadii.control),
              focusNode: _focusNode,
              onTap: widget.onTap,
              onLongPress: () => _menuController.open(),
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
class const _SessionStatusMark({
  required final SessionId sessionId,
  required final bool running,
}) extends StatelessWidget {
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

enum _NewSessionAction { here, folder }

/// One row of the new-session menu.
class const _NewSessionMenuItem({
  required final IconData icon,
  required final String label,
}) extends StatelessWidget {
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
class const _SidebarActionButton({
  super.key,
  required final IconData icon,
  required final String label,
  final VoidCallback? onTap,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: Material(
        type: MaterialType.transparency,
        child: InkWell(
          focusColor: colors.raised,
          borderRadius: BorderRadius.circular(AtlasRadii.control),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
            child: Row(
              children: [
                Icon(icon, size: 14, color: colors.textPrimary),
                const SizedBox(width: 12),
                Text(
                  label,
                  style: TextStyle(
                    color: colors.textPrimary,
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../app/acp_connections.dart';
import '../../../../app/runtime_environment.dart';
import '../../../../shared/theme/atlas_theme.dart';
import 'workspace_controls.dart';

/// Settings dialog for managing ACP server connections.
///
/// Lists saved connections, lets the user add or remove them, and activates
/// one to switch the runtime from the local agent to a remote ACP server.
class SettingsDialog extends ConsumerStatefulWidget {
  /// Creates a settings dialog.
  const SettingsDialog({super.key});

  @override
  ConsumerState<SettingsDialog> createState() => _SettingsDialogState();
}

class _SettingsDialogState extends ConsumerState<SettingsDialog> {
  final _connections = <AcpConnection>[];

  @override
  void initState() {
    super.initState();
    _connections.addAll(loadAcpConnections());
  }

  void _save() => saveAcpConnections(_connections);

  Future<void> _addConnection() async {
    final connection = await showDialog<AcpConnection>(
      context: context,
      builder: (context) => const _ConnectionFormDialog(),
    );
    if (connection != null) {
      setState(() => _connections.add(connection));
      _save();
    }
  }

  Future<void> _removeConnection(AcpConnection connection) async {
    setState(() => _connections.remove(connection));
    _save();
  }

  Future<void> _activate(AcpConnection connection) async {
    final controller = ref.read(runtimeEnvironmentProvider.notifier);
    try {
      await controller.activateConnection(connection);
    } on StateError {
      // The failure is surfaced through acpConnectionErrorProvider.
    }
  }

  Future<void> _deactivate() async {
    await ref.read(runtimeEnvironmentProvider.notifier).deactivateConnection();
  }

  bool _isActive(AcpRuntimeState state, AcpConnection connection) {
    final active = state.activeConnection;
    if (state.status != AcpConnectionStatus.connected || active == null) {
      return false;
    }
    return active.name == connection.name &&
        active.command == connection.command &&
        _sameArguments(active.arguments, connection.arguments);
  }

  static bool _sameArguments(List<String> left, List<String> right) {
    if (left.length != right.length) {
      return false;
    }
    for (var i = 0; i < left.length; i++) {
      if (left[i] != right[i]) {
        return false;
      }
    }
    return true;
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final runtimeState = ref.watch(runtimeEnvironmentProvider);
    final status = runtimeState.status;
    final error = runtimeState.activationError;
    return AlertDialog(
      title: const Text('Settings'),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'ACP Connections',
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12,
                fontWeight: FontWeight.w600,
              ),
            ),
            const SizedBox(height: 8),
            if (_connections.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: Text(
                  'No connections yet. Add one to use a remote agent.',
                  style: TextStyle(color: colors.textSecondary, fontSize: 12),
                ),
              )
            else
              for (final connection in _connections)
                _ConnectionRow(
                  connection: connection,
                  active: _isActive(runtimeState, connection),
                  onActivate: () => unawaited(_activate(connection)),
                  onRemove: () => unawaited(_removeConnection(connection)),
                ),
            if (error != null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  error,
                  style: TextStyle(color: colors.error, fontSize: 12),
                ),
              ),
            const SizedBox(height: 8),
            if (status == AcpConnectionStatus.connected)
              WorkspaceHoverSurface(
                borderRadius: BorderRadius.circular(AtlasRadii.control),
                child: TextButton.icon(
                  onPressed: () => unawaited(_deactivate()),
                  icon: const Icon(LucideIcons.rotateCcw, size: 14),
                  label: const Text('Back to local runtime'),
                ),
              ),
            WorkspaceHoverSurface(
              borderRadius: BorderRadius.circular(AtlasRadii.control),
              child: TextButton.icon(
                onPressed: () => unawaited(_addConnection()),
                icon: const Icon(LucideIcons.plus, size: 14),
                label: const Text('Add connection'),
              ),
            ),
          ],
        ),
      ),
      actions: [
        WorkspaceHoverSurface(
          borderRadius: BorderRadius.circular(AtlasRadii.control),
          child: TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
        ),
      ],
    );
  }
}

/// One saved connection with activate and remove actions.
class _ConnectionRow extends StatelessWidget {
  const _ConnectionRow({
    required this.connection,
    required this.active,
    required this.onActivate,
    required this.onRemove,
  });

  final AcpConnection connection;
  final bool active;
  final VoidCallback onActivate;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          Icon(
            active ? LucideIcons.plugZap : LucideIcons.plug,
            size: 14,
            color: active ? colors.success : colors.textSecondary,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  connection.name,
                  style: TextStyle(
                    color: colors.textPrimary,
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                  ),
                ),
                Text(
                  '${connection.command} ${connection.arguments.join(' ')}',
                  style: TextStyle(color: colors.textSecondary, fontSize: 11),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
          if (!active)
            WorkspaceHoverSurface(
              borderRadius: BorderRadius.circular(AtlasRadii.control),
              child: TextButton(
                onPressed: onActivate,
                child: const Text('Activate'),
              ),
            ),
          WorkspaceHoverSurface(
            borderRadius: BorderRadius.circular(AtlasRadii.control),
            child: IconButton(
              icon: const Icon(LucideIcons.trash2, size: 14),
              tooltip: 'Remove connection',
              onPressed: onRemove,
            ),
          ),
        ],
      ),
    );
  }
}

/// Form for creating or editing a connection, with preset shortcuts.
class _ConnectionFormDialog extends ConsumerStatefulWidget {
  const _ConnectionFormDialog();

  @override
  ConsumerState<_ConnectionFormDialog> createState() =>
      _ConnectionFormDialogState();
}

class _ConnectionFormDialogState extends ConsumerState<_ConnectionFormDialog> {
  final _name = TextEditingController();
  final _command = TextEditingController();
  final _arguments = TextEditingController();

  @override
  void dispose() {
    _name.dispose();
    _command.dispose();
    _arguments.dispose();
    super.dispose();
  }

  void _applyPreset(AcpConnection preset) {
    setState(() {
      _name.text = preset.name;
      _command.text = preset.command;
      _arguments.text = preset.arguments.join(' ');
    });
  }

  void _submit() {
    final name = _name.text.trim();
    final command = _command.text.trim();
    if (name.isEmpty || command.isEmpty) {
      return;
    }
    final arguments = _arguments.text
        .trim()
        .split(RegExp(r'\s+'))
        .where((argument) => argument.isNotEmpty)
        .toList();
    Navigator.pop(
      context,
      AcpConnection(name: name, command: command, arguments: arguments),
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return AlertDialog(
      title: const Text('Add ACP Connection'),
      content: SizedBox(
        width: 380,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Presets',
              style: TextStyle(color: colors.textSecondary, fontSize: 12),
            ),
            const SizedBox(height: 6),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                for (final preset in acpPresets)
                  WorkspaceHoverSurface(
                    borderRadius: BorderRadius.circular(AtlasRadii.control),
                    child: ActionChip(
                      label: Text(preset.name),
                      onPressed: () => _applyPreset(preset),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _name,
              autofocus: true,
              decoration: const InputDecoration(labelText: 'Name'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _command,
              decoration: const InputDecoration(labelText: 'Command'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _arguments,
              decoration: const InputDecoration(
                labelText: 'Arguments (space separated)',
              ),
            ),
          ],
        ),
      ),
      actions: [
        WorkspaceHoverSurface(
          borderRadius: BorderRadius.circular(AtlasRadii.control),
          child: TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
        ),
        WorkspaceHoverSurface(
          borderRadius: BorderRadius.circular(AtlasRadii.control),
          child: TextButton(
            onPressed: _submit,
            child: Text('Add', style: TextStyle(color: colors.accent)),
          ),
        ),
      ],
    );
  }
}

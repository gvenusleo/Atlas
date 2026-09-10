import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../app/remote_connections.dart';
import '../../../../app/runtime_environment.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_controller.dart';

/// Prompts for the computer-side working directory of a remote connection.
///
/// ACP creates sessions with a mandatory `cwd`, but requiring the directory
/// when the profile is created is friction the connection does not need:
/// history and model selection work without it. This bar appears above the
/// composer while the focused remote draft has no directory yet, and stores
/// the choice back into the profile so later connections skip the prompt.
class RemoteWorkingDirectoryBar extends ConsumerStatefulWidget {
  /// Creates a directory prompt bound to the focused workspace draft.
  const RemoteWorkingDirectoryBar({super.key});

  @override
  ConsumerState<RemoteWorkingDirectoryBar> createState() =>
      _RemoteWorkingDirectoryBarState();
}

class _RemoteWorkingDirectoryBarState
    extends ConsumerState<RemoteWorkingDirectoryBar> {
  final _store = RemoteConnectionStore();
  bool _saving = false;

  @override
  Widget build(BuildContext context) {
    final runtimeState = ref.watch(runtimeEnvironmentProvider);
    final environment = runtimeState.environment;
    final profile = runtimeState.remoteProfile;
    if (environment?.isRemote != true ||
        profile?.workingDirectory != null ||
        profile == null) {
      return const SizedBox.shrink();
    }
    // Only drafts need the directory: resuming an existing session keeps the
    // directory the session was created in.
    final focused = ref.watch(
      workspaceProvider.select((s) => s.workspaces[s.activeKey]),
    );
    if (focused == null || focused.sessionId != null) {
      return const SizedBox.shrink();
    }
    final colors = AtlasColors.of(context);
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 10, 24, 0),
      child: Container(
        key: const ValueKey('atlas-remote-directory-bar'),
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(
          color: colors.canvas,
          borderRadius: BorderRadius.circular(AtlasRadii.surface),
          border: Border.all(color: colors.divider),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(LucideIcons.folderOpen, size: 14, color: colors.accent),
            const SizedBox(width: 8),
            Flexible(
              child: Text(
                'Sessions run in a directory on your computer',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(color: colors.textSecondary, fontSize: 12),
              ),
            ),
            const SizedBox(width: 10),
            if (_saving)
              const SizedBox.square(
                dimension: 12,
                child: CircularProgressIndicator(strokeWidth: 1.5),
              )
            else
              TextButton(
                key: const ValueKey('atlas-remote-set-directory'),
                onPressed: _chooseDirectory,
                style: TextButton.styleFrom(
                  visualDensity: VisualDensity.compact,
                  minimumSize: const Size(0, 28),
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  textStyle: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                child: const Text('Choose directory'),
              ),
          ],
        ),
      ),
    );
  }

  Future<void> _chooseDirectory() async {
    final path = await showDialog<String>(
      context: context,
      builder: (context) => const _RemoteDirectoryDialog(),
    );
    if (path == null || !mounted) {
      return;
    }
    final controller = ref.read(runtimeEnvironmentProvider.notifier);
    controller.setRemoteWorkingDirectory(path);
    setState(() => _saving = true);
    try {
      await _persist(path);
    } finally {
      if (mounted) {
        setState(() => _saving = false);
      }
    }
    if (!mounted) {
      return;
    }
    // Fresh draft rooted at the chosen directory; identical to switching
    // directories on the desktop.
    ref.read(workspaceProvider.notifier).newSession(workingDirectory: path);
  }

  /// Stores the directory on the matching stored profile (best effort: the
  /// in-memory profile is already updated, persistence failures only cost a
  /// repeated prompt on the next connection).
  Future<void> _persist(String directory) async {
    final profile = ref.read(runtimeEnvironmentProvider).remoteProfile;
    if (profile == null) {
      return;
    }
    try {
      final profiles = await _store.load();
      final index = profiles.indexWhere(
        (entry) => entry.name == profile.name && entry.wsUrl == profile.wsUrl,
      );
      if (index < 0) {
        return;
      }
      profiles[index] = profiles[index].copyWith(workingDirectory: directory);
      await _store.save(profiles);
    } on Object catch (error) {
      debugPrint('Cannot persist remote working directory: $error');
    }
  }
}

class _RemoteDirectoryDialog extends StatefulWidget {
  const _RemoteDirectoryDialog();

  @override
  State<_RemoteDirectoryDialog> createState() => _RemoteDirectoryDialogState();
}

class _RemoteDirectoryDialogState extends State<_RemoteDirectoryDialog> {
  final _controller = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _submit() {
    final path = _controller.text.trim();
    if (path.isEmpty) {
      setState(() => _error = 'Enter an absolute path such as /home/you.');
      return;
    }
    if (!path.startsWith('/')) {
      setState(() => _error = 'Absolute paths start with a /.');
      return;
    }
    Navigator.of(context).pop(path);
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return AlertDialog(
      title: const Text('Working directory on the computer'),
      content: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              'New sessions run in this directory on the computer (for '
              'example /home/you/projects).',
              style: TextStyle(
                color: colors.textSecondary,
                fontSize: 12.5,
                height: 1.5,
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _controller,
              autofocus: true,
              decoration: InputDecoration(
                labelText: 'Directory',
                hintText: '/home/you/projects',
                errorText: _error,
                isDense: true,
                border: const OutlineInputBorder(),
              ),
              onSubmitted: (_) => _submit(),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(onPressed: _submit, child: const Text('Use directory')),
      ],
    );
  }
}

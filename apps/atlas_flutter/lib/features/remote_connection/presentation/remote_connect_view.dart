import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import '../data/remote_connections.dart';
import '../application/runtime_controller.dart';
import '../application/connection_profiles_controller.dart';
import '../../../shared/theme/atlas_theme.dart';
import '../../workspace/presentation/widgets/workspace_controls.dart';
import 'remote_profile_form.dart';

/// Entry state for mobile clients and fallback for desktop without a runtime:
/// manages remote server profiles and drives the connection lifecycle.
class const RemoteConnectView({super.key}) extends ConsumerStatefulWidget {
  @override
  ConsumerState<RemoteConnectView> createState() => _RemoteConnectViewState();
}

class _RemoteConnectViewState extends ConsumerState<RemoteConnectView> {
  bool _busy = false;

  /// Surfaces a persistence failure; without this the dialog closes and the
  /// list stays unchanged with no explanation (for example a locked system
  /// keyring on Linux or an unavailable Keystore).
  Future<void> _showSaveError(Object error) async {
    if (!mounted) {
      return;
    }
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Could not save the connection'),
        content: Text('$error'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }

  Future<void> _addOrEdit([RemoteConnectionProfile? profile]) async {
    final result = await showDialog<RemoteConnectionProfile>(
      context: context,
      builder: (context) => RemoteProfileFormDialog(profile: profile),
    );
    if (result == null || !mounted) {
      return;
    }
    try {
      await ref
          .read(remoteProfilesProvider.notifier)
          .saveProfile(result, previous: profile);
    } on Object catch (error) {
      await _showSaveError(error);
    }
  }

  Future<void> _remove(RemoteConnectionProfile profile) async {
    try {
      await ref.read(remoteProfilesProvider.notifier).removeProfile(profile);
    } on Object catch (error) {
      await _showSaveError(error);
    }
  }

  Future<void> _connect(RemoteConnectionProfile profile) async {
    final controller = ref.read(runtimeEnvironmentProvider.notifier);
    setState(() => _busy = true);
    try {
      // The controller applies the profile's computer-side working directory
      // only after the connection succeeds.
      await controller.activateRemote(profile);
    } catch (error) {
      if (mounted) {
        setState(() => _busy = false);
        await showDialog<void>(
          context: context,
          builder: (context) => AlertDialog(
            title: const Text('Connection failed'),
            content: Text('$error'),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: const Text('OK'),
              ),
            ],
          ),
        );
      }
    } finally {
      if (mounted) {
        setState(() => _busy = false);
      }
    }
  }

  Future<void> _disconnect() async {
    await ref.read(runtimeEnvironmentProvider.notifier).disconnectRemote();
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final runtimeState = ref.watch(runtimeEnvironmentProvider);
    final savedProfiles = ref.watch(remoteProfilesProvider);
    final profiles = savedProfiles.value ?? const <RemoteConnectionProfile>[];
    final loadError = savedProfiles.error;
    final status = runtimeState.remoteStatus;

    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 560),
        child: CustomScrollView(
          slivers: [
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(24, 24, 24, 0),
              sliver: SliverToBoxAdapter(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Row(
                      children: [
                        Icon(
                          LucideIcons.monitorSmartphone,
                          color: colors.accent,
                        ),
                        const SizedBox(width: 10),
                        Expanded(
                          child: Text(
                            'Connect to the Atlas on your computer',
                            style: TextStyle(
                              color: colors.textPrimary,
                              fontSize: 16,
                              fontWeight: FontWeight.w600,
                            ),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 6),
                    Text(
                      'Run `atlas server` on the computer and enter its address '
                      'and token below. Models and commands run on the computer; '
                      'the working directory for new sessions is chosen right '
                      'before the first message.',
                      style: TextStyle(
                        color: colors.textSecondary,
                        fontSize: 12.5,
                        height: 1.5,
                      ),
                    ),
                    const SizedBox(height: 16),
                    if (loadError != null) ...[
                      Text(
                        '$loadError',
                        style: TextStyle(color: colors.error, fontSize: 12),
                      ),
                      const SizedBox(height: 8),
                    ],
                    if (profiles.isEmpty && !_busy)
                      Padding(
                        padding: const EdgeInsets.symmetric(vertical: 8),
                        child: Text(
                          'No connections yet.',
                          style: TextStyle(
                            color: colors.textSecondary,
                            fontSize: 12.5,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
            SliverPadding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              sliver: SliverList.builder(
                itemCount: profiles.length,
                itemBuilder: (context, index) {
                  final profile = profiles[index];
                  return _ProfileTile(
                    profile: profile,
                    active:
                        runtimeState.remoteProfile?.wsUrl == profile.wsUrl &&
                        runtimeState.remoteProfile?.name == profile.name,
                    status:
                        runtimeState.remoteProfile?.wsUrl == profile.wsUrl &&
                            runtimeState.remoteProfile?.name == profile.name
                        ? status
                        : RemoteConnectionStatus.disconnected,
                    error: runtimeState.remoteProfile?.wsUrl == profile.wsUrl
                        ? runtimeState.remoteError
                        : null,
                    busy: _busy,
                    onConnect: () => unawaited(_connect(profile)),
                    onDisconnect: () => unawaited(_disconnect()),
                    onEdit: () => unawaited(_addOrEdit(profile)),
                    onRemove: () => unawaited(_remove(profile)),
                  );
                },
              ),
            ),
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(24, 12, 24, 24),
              sliver: SliverToBoxAdapter(
                child: OutlinedButton.icon(
                  onPressed: _busy ? null : () => unawaited(_addOrEdit()),
                  icon: const Icon(LucideIcons.plus, size: 16),
                  label: const Text('Add connection'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// One saved profile row with its connection state.
class const _ProfileTile({
  required final RemoteConnectionProfile profile,
  required final bool active,
  required final RemoteConnectionStatus status,
  required final String? error,
  required final bool busy,
  required final VoidCallback onConnect,
  required final VoidCallback onDisconnect,
  required final VoidCallback onEdit,
  required final VoidCallback onRemove,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final (label, color) = switch (status) {
      RemoteConnectionStatus.connecting => ('Connecting…', colors.accent),
      RemoteConnectionStatus.connected => ('Connected', colors.success),
      RemoteConnectionStatus.reconnecting => ('Reconnecting…', colors.warning),
      RemoteConnectionStatus.error => ('Connection failed', colors.error),
      RemoteConnectionStatus.disconnected => (
        'Not connected',
        colors.textSecondary,
      ),
    };
    return WorkspaceHoverSurface(
      borderRadius: BorderRadius.circular(AtlasRadii.control),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(10, 8, 6, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    color: color,
                    shape: BoxShape.circle,
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    profile.name,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      color: colors.textPrimary,
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                ),
                IconButton(
                  visualDensity: VisualDensity.compact,
                  tooltip: 'Edit',
                  icon: const Icon(LucideIcons.pencil, size: 14),
                  onPressed: busy ? null : onEdit,
                ),
                IconButton(
                  visualDensity: VisualDensity.compact,
                  tooltip: 'Remove',
                  icon: const Icon(LucideIcons.trash, size: 14),
                  onPressed: busy ? null : onRemove,
                ),
              ],
            ),
            Padding(
              padding: const EdgeInsets.only(left: 16),
              child: Text(
                profile.wsUrl,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: colors.textSecondary,
                  fontSize: 11.5,
                  fontFamily: 'monospace',
                ),
              ),
            ),
            if (error != null && error!.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(left: 16, top: 4),
                child: Text(
                  error!,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: colors.error, fontSize: 11.5),
                ),
              )
            else
              Padding(
                padding: const EdgeInsets.only(left: 16, top: 4),
                child: Text(
                  label,
                  style: TextStyle(color: color, fontSize: 11.5),
                ),
              ),
            Align(
              alignment: Alignment.centerRight,
              child: active && status == RemoteConnectionStatus.connected
                  ? TextButton(
                      onPressed: busy ? null : onDisconnect,
                      child: const Text('Disconnect'),
                    )
                  : TextButton(
                      onPressed: busy ? null : onConnect,
                      child: Text(
                        status == RemoteConnectionStatus.error
                            ? 'Retry'
                            : 'Connect',
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }
}

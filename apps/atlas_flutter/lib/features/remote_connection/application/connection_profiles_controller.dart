import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/acp_connections.dart';
import '../data/connection_repository.dart';
import '../data/remote_connections.dart';
import 'runtime_controller.dart';

/// Shared remote profile repository, replaceable with an in-memory store.
final remoteConnectionRepositoryProvider = Provider(
  (ref) =>
      ConnectionRepository<RemoteConnectionProfile>(RemoteConnectionStore()),
);

/// Shared ACP connection repository.
final acpConnectionRepositoryProvider = Provider(
  (ref) => ConnectionRepository<AcpConnection>(const AcpConnectionStore()),
);

/// Saved remote profiles observed by every connection surface.
final remoteProfilesProvider =
    AsyncNotifierProvider<
      RemoteProfilesController,
      List<RemoteConnectionProfile>
    >(RemoteProfilesController.new);

/// Coordinates remote profile edits without exposing storage to widgets.
class RemoteProfilesController
    extends AsyncNotifier<List<RemoteConnectionProfile>> {
  ConnectionRepository<RemoteConnectionProfile> get _repository =>
      ref.read(remoteConnectionRepositoryProvider);

  @override
  Future<List<RemoteConnectionProfile>> build() =>
      ref.watch(remoteConnectionRepositoryProvider).load();

  Future<void> _update(
    void Function(List<RemoteConnectionProfile>) edit,
  ) async {
    final profiles = await _repository.update(edit);
    if (ref.mounted) state = AsyncData(profiles);
  }

  /// Adds a profile or replaces [previous], including renamed profiles.
  Future<void> saveProfile(
    RemoteConnectionProfile profile, {
    RemoteConnectionProfile? previous,
  }) => _update((profiles) {
    final key = previous ?? profile;
    final index = profiles.indexWhere((entry) => _same(entry, key));
    if (index < 0) {
      profiles.add(profile);
    } else {
      profiles[index] = profile;
    }
  });

  /// Removes a saved profile, disconnecting it first when it is active.
  Future<void> removeProfile(RemoteConnectionProfile profile) async {
    final active = ref.read(runtimeEnvironmentProvider).remoteProfile;
    if (active != null && _same(active, profile)) {
      await ref.read(runtimeEnvironmentProvider.notifier).disconnectRemote();
    }
    await _update(
      (profiles) => profiles.removeWhere((entry) => _same(entry, profile)),
    );
  }

  /// Updates only the directory on the latest saved version of [profile].
  Future<void> setWorkingDirectory(
    RemoteConnectionProfile profile,
    String directory,
  ) => _update((profiles) {
    final index = profiles.indexWhere((entry) => _same(entry, profile));
    if (index >= 0) {
      profiles[index] = profiles[index].copyWith(workingDirectory: directory);
    }
  });

  bool _same(RemoteConnectionProfile left, RemoteConnectionProfile right) =>
      left.name == right.name && left.wsUrl == right.wsUrl;
}

/// Saved ACP subprocess connections observed by settings.
final acpConnectionsProvider =
    AsyncNotifierProvider<AcpConnectionsController, List<AcpConnection>>(
      AcpConnectionsController.new,
    );

/// Coordinates ACP profile edits and publishes only successfully saved lists.
class AcpConnectionsController extends AsyncNotifier<List<AcpConnection>> {
  ConnectionRepository<AcpConnection> get _repository =>
      ref.read(acpConnectionRepositoryProvider);

  @override
  Future<List<AcpConnection>> build() =>
      ref.watch(acpConnectionRepositoryProvider).load();

  /// Saves a new subprocess connection.
  Future<void> add(AcpConnection connection) async {
    final connections = await _repository.update(
      (items) => items.add(connection),
    );
    if (ref.mounted) state = AsyncData(connections);
  }

  /// Removes a saved subprocess connection.
  Future<void> remove(AcpConnection connection) async {
    final connections = await _repository.update(
      (items) => items.remove(connection),
    );
    if (ref.mounted) state = AsyncData(connections);
  }
}

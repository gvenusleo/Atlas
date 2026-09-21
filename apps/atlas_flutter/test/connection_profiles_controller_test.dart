import 'dart:async';

import 'package:atlas_flutter/features/remote_connection/application/connection_profiles_controller.dart';
import 'package:atlas_flutter/features/remote_connection/data/acp_connections.dart';
import 'package:atlas_flutter/features/remote_connection/data/connection_repository.dart';
import 'package:atlas_flutter/features/remote_connection/data/remote_connections.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  const first = RemoteConnectionProfile(
    name: 'first',
    wsUrl: 'ws://first/acp',
    token: 'one',
  );
  const second = RemoteConnectionProfile(
    name: 'second',
    wsUrl: 'ws://second/acp',
    token: 'two',
  );

  test(
    'repository serializes updates and exposes immutable snapshots',
    () async {
      final store = _Store<RemoteConnectionProfile>([]);
      final gate = Completer<void>();
      store.writeGate = gate.future;
      final repository = ConnectionRepository(store);
      final a = repository.update((items) => items.add(first));
      final b = repository.update((items) => items.add(second));
      await pumpEventQueue();
      expect(store.writes, 1);
      gate.complete();
      await a;
      final result = await b;
      expect(result, [first, second]);
      expect(store.items, result);
      expect(() => result.clear(), throwsUnsupportedError);
      expect(store.reads, 1);
    },
  );

  test('failed writes preserve the snapshot and allow later updates', () async {
    final store = _Store<RemoteConnectionProfile>([first]);
    final repository = ConnectionRepository(store);
    await repository.load();
    store.failWrite = true;
    await expectLater(
      repository.update((items) => items.clear()),
      throwsStateError,
    );
    expect(await repository.load(), [first]);
    store.failWrite = false;
    expect(await repository.update((items) => items.add(second)), [
      first,
      second,
    ]);
  });

  test(
    'profile edits and directory updates share the latest saved state',
    () async {
      final store = _Store<RemoteConnectionProfile>([first]);
      final container = ProviderContainer(
        overrides: [
          remoteConnectionRepositoryProvider.overrideWithValue(
            ConnectionRepository(store),
          ),
        ],
      );
      addTearDown(container.dispose);
      await container.read(remoteProfilesProvider.future);
      final controller = container.read(remoteProfilesProvider.notifier);
      const edited = RemoteConnectionProfile(
        name: 'renamed',
        wsUrl: 'ws://first/acp',
        token: 'new',
      );
      await controller.saveProfile(edited, previous: first);
      await Future.wait([
        controller.setWorkingDirectory(edited, '/project'),
        controller.saveProfile(second),
      ]);
      final profiles = container.read(remoteProfilesProvider).requireValue;
      expect(profiles, hasLength(2));
      expect(profiles.first.name, 'renamed');
      expect(profiles.first.token, 'new');
      expect(profiles.first.workingDirectory, '/project');
      expect(profiles.last, second);
      expect(store.items, profiles);
      await controller.removeProfile(second);
      expect(container.read(remoteProfilesProvider).requireValue, hasLength(1));
    },
  );

  test(
    'ACP controller publishes additions and removals only after saving',
    () async {
      final store = _Store<AcpConnection>([]);
      final container = ProviderContainer(
        overrides: [
          acpConnectionRepositoryProvider.overrideWithValue(
            ConnectionRepository(store),
          ),
        ],
      );
      addTearDown(container.dispose);
      await container.read(acpConnectionsProvider.future);
      final controller = container.read(acpConnectionsProvider.notifier);
      await controller.add(atlasPreset);
      store.failWrite = true;
      await expectLater(controller.remove(atlasPreset), throwsStateError);
      expect(container.read(acpConnectionsProvider).requireValue, [
        atlasPreset,
      ]);
      store.failWrite = false;
      await controller.remove(atlasPreset);
      expect(container.read(acpConnectionsProvider).requireValue, isEmpty);
    },
  );
}

class _Store<T>(var List<T> items) implements ConnectionStore<T> {
  int reads = 0;
  int writes = 0;
  bool failWrite = false;
  Future<void>? writeGate;

  @override
  Future<List<T>> load() async {
    reads++;
    return List.of(items);
  }

  @override
  Future<void> save(List<T> connections) async {
    writes++;
    await writeGate;
    if (failWrite) throw StateError('write failed');
    items = List.of(connections);
  }
}

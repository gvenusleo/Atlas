import 'package:atlas_flutter/app/remote_connections.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    FlutterSecureStorage.setMockInitialValues({});
  });

  test('profile json round-trips without the token', () {
    const profile = RemoteConnectionProfile(
      name: 'My computer',
      wsUrl: 'ws://host:8765/acp',
      token: 'secret-token',
      workingDirectory: '/home/you/projects',
    );
    final json = profile.toJson();
    expect(json['token'], isNull);
    final restored = RemoteConnectionProfile.fromJson({
      ...json,
      'token': 'secret-token',
    });
    expect(restored, isNotNull);
    expect(restored!.name, 'My computer');
    expect(restored.token, 'secret-token');
    expect(restored.workingDirectory, '/home/you/projects');
  });

  test('store persists profiles including tokens in secure storage', () async {
    final store = RemoteConnectionStore();
    await store.save(const [
      RemoteConnectionProfile(
        name: 'One',
        wsUrl: 'ws://a/acp',
        token: 'token-a',
      ),
      RemoteConnectionProfile(
        name: 'Two',
        wsUrl: 'ws://b/acp',
        token: 'token-b',
        workingDirectory: '/srv',
      ),
    ]);
    final loaded = await store.load();
    expect(loaded, hasLength(2));
    expect(loaded[0].name, 'One');
    expect(loaded[0].token, 'token-a');
    expect(loaded[1].token, 'token-b');
    expect(loaded[1].workingDirectory, '/srv');
  });

  test('store tolerates missing and malformed values', () async {
    final store = RemoteConnectionStore();
    expect(await store.load(), isEmpty);

    FlutterSecureStorage.setMockInitialValues({
      'atlas.remote_connections': 'not json',
    });
    expect(await store.load(), isEmpty);
  });

  test('an empty load returns a growable list for the first save', () async {
    // Regression: the connect view merges the new profile into the loaded
    // list and adds to it; `const []` made the very first save fail with
    // "Cannot add to an unmodifiable list".
    final store = RemoteConnectionStore();
    final profiles = await store.load();
    profiles.add(
      const RemoteConnectionProfile(
        name: 'First',
        wsUrl: 'ws://a/acp',
        token: 'token-a',
      ),
    );
    await store.save(profiles);
    final loaded = await store.load();
    expect(loaded, hasLength(1));
    expect(loaded.single.name, 'First');
  });

  test('fromJson rejects malformed profiles', () {
    expect(RemoteConnectionProfile.fromJson(null), isNull);
    expect(RemoteConnectionProfile.fromJson('nope'), isNull);
    expect(RemoteConnectionProfile.fromJson({'name': 'x'}), isNull);
    expect(
      RemoteConnectionProfile.fromJson({
        'name': 'x',
        'wsUrl': 'ws://x/acp',
        'token': 42,
      })!.token,
      isEmpty,
    );
  });

  test('fromJson requires the name and wsUrl keys to hold strings', () {
    // A map pattern matches on key presence, so an omitted key, an explicit
    // null, a wrong type, and an empty string must all be rejected.
    for (final json in <Object?>[
      <String, Object?>{'wsUrl': 'ws://x/acp'},
      <String, Object?>{'name': 'x'},
      <String, Object?>{'name': 'x', 'wsUrl': null},
      <String, Object?>{'name': null, 'wsUrl': 'ws://x/acp'},
      <String, Object?>{'name': 7, 'wsUrl': 'ws://x/acp'},
      <String, Object?>{'name': 'x', 'wsUrl': 7},
      <String, Object?>{'name': '', 'wsUrl': 'ws://x/acp'},
      <String, Object?>{'name': 'x', 'wsUrl': ''},
    ]) {
      expect(
        RemoteConnectionProfile.fromJson(json),
        isNull,
        reason: 'for $json',
      );
    }
  });

  test('fromJson ignores unknown keys and defaults the optional fields', () {
    final profile = RemoteConnectionProfile.fromJson(<String, Object?>{
      'name': 'x',
      'wsUrl': 'ws://x/acp',
      'token': 'token-a',
      'unexpected': true,
    });

    expect(profile, isNotNull);
    expect(profile!.name, 'x');
    expect(profile.token, 'token-a');
    expect(profile.workingDirectory, isNull);
    // An explicitly present but non-string directory falls back to null
    // instead of failing the whole profile.
    expect(
      RemoteConnectionProfile.fromJson(<String, Object?>{
        'name': 'x',
        'wsUrl': 'ws://x/acp',
        'workingDirectory': '',
      })!.workingDirectory,
      isNull,
    );
  });
}

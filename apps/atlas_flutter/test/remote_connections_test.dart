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
}

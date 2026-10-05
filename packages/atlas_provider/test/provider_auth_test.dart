import 'dart:io';

import 'package:atlas_provider/atlas_provider.dart';
import 'package:test/test.dart';

void main() {
  late Directory dir;
  late AuthStore store;
  setUp(() async {
    dir = await Directory.systemTemp.createTemp('atlas_auth_');
    store = AuthStore(File('${dir.path}/auth.json'));
  });
  tearDown(() => dir.delete(recursive: true));

  test(
    'writes merge independent providers and remove only the selected key',
    () async {
      await Future.wait([store.setKey('a', 'one'), store.setKey('b', 'two')]);
      expect(store.readKey('a'), 'one');
      expect(store.readKey('b'), 'two');
      await store.setKey('a', null);
      expect(store.readKey('a'), isNull);
      expect(store.readKey('b'), 'two');
      if (!Platform.isWindows) {
        expect((await store.file.stat()).mode & 0x1ff, 0x180);
      }
    },
  );

  test(
    'request-time resolution follows Pi precedence and observes key changes',
    () async {
      final resolver = ConfigValueResolver({
        'KEY': 'configured',
        'ENV': 'ambient',
      });
      ProviderAuthentication auth({
        String? runtime,
        String? configured = r'${KEY}',
      }) => ProviderAuthentication(
        provider: 'relay',
        resolver: resolver,
        store: store,
        apiKey: configured,
        runtimeApiKey: runtime,
        environmentKeys: ['ENV'],
      );
      expect((await auth(configured: null).resolve()).key, 'ambient');
      expect((await auth().resolve()).key, 'configured');
      await store.setKey('relay', 'stored');
      expect((await auth().resolve()).key, 'stored');
      expect((await auth(runtime: 'runtime').resolve()).key, 'runtime');
      await store.setKey('relay', 'rotated');
      expect((await auth().resolve()).key, 'rotated');
    },
  );

  test(
    'interpolation escapes and missing secrets have redacted failures',
    () async {
      final resolver = ConfigValueResolver({'KEY': 'secret'});
      expect(
        await resolver.resolve(r'Bearer $KEY/${KEY}/$$/$!'),
        r'Bearer secret/secret/$/!',
      );
      expect(
        resolver.resolve(r'${MISSING}'),
        throwsA(
          isA<ProviderAuthException>().having(
            (e) => e.safeMessage,
            'message',
            contains('MISSING'),
          ),
        ),
      );
      final auth = ProviderAuthentication(
        provider: 'relay',
        resolver: ConfigValueResolver({'KEY': 'secret\r\nInjected: yes'}),
        headers: {'authorization': r'Bearer ${KEY}'},
      );
      expect(
        auth.resolve(),
        throwsA(
          isA<ProviderAuthException>().having(
            (e) => e.safeMessage,
            'message',
            isNot(contains('secret')),
          ),
        ),
      );
    },
  );

  test(
    'command credentials are re-evaluated and failures omit commands',
    () async {
      final resolver = ConfigValueResolver({
        ...Platform.environment,
        'AUTH_VALUE': 'command-key',
      });
      final command = Platform.isWindows
          ? '!echo %AUTH_VALUE%'
          : r'!printf "%s" "$AUTH_VALUE"';
      expect(await resolver.resolve(command), 'command-key');
      expect(await resolver.resolve(command), 'command-key');
      expect(
        resolver.resolve('!exit 7'),
        throwsA(isA<ProviderAuthException>()),
      );
    },
  );

  test('malformed auth is not overwritten by a save', () async {
    await store.file.writeAsString('{broken');
    await expectLater(
      store.setKey('relay', 'key'),
      throwsA(isA<ProviderAuthException>()),
    );
    await expectLater(
      store.setKey('other', 'key'),
      throwsA(isA<ProviderAuthException>()),
    );
    expect(await store.file.readAsString(), '{broken');
  });
}

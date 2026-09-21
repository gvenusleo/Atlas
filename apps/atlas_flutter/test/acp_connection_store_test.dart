import 'dart:io';

import 'package:atlas_flutter/features/remote_connection/data/acp_connections.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory home;
  late AcpConnectionStore store;

  setUp(() async {
    home = await Directory.systemTemp.createTemp('atlas_acp_connections_test');
    store = AcpConnectionStore(home: home.path);
    addTearDown(() => home.delete(recursive: true));
  });

  test('asynchronously persists and reloads subprocess connections', () async {
    expect(await store.load(), isEmpty);
    await store.save([atlasPreset, codexPreset]);
    final restored = await store.load();
    expect(restored.map((item) => item.name), ['Atlas', 'Codex']);
    expect(restored.last.command, 'npx');
    expect(restored.last.arguments, codexPreset.arguments);
    await store.save([]);
    expect(await store.load(), isEmpty);
  });

  test('malformed configuration produces an empty list', () async {
    await store.save([]);
    await File(
      '${home.path}/.atlas/acp_connections.json',
    ).writeAsString('not json');
    expect(await store.load(), isEmpty);
    await store.save([atlasPreset]);
    expect(await store.load(), hasLength(1));
  });

  test(
    'write errors reach the controller instead of reporting success',
    () async {
      await File('${home.path}/.atlas').writeAsString('not a directory');
      await expectLater(
        store.save([atlasPreset]),
        throwsA(isA<FileSystemException>()),
      );
    },
  );
}

import 'dart:io';

import 'package:atlas_ws/atlas_ws.dart';
import 'package:test/test.dart';

void main() {
  late Directory temp;
  late File tokenFile;
  late RemoteTokenFile tokens;

  setUp(() async {
    temp = await Directory.systemTemp.createTemp('atlas_ws_token_test');
    tokenFile = File('${temp.path}/remote_token');
    tokens = RemoteTokenFile(tokenFile);
  });

  tearDown(() async {
    await temp.delete(recursive: true);
  });

  test('generateToken produces a 256-bit URL-safe value', () {
    final token = generateToken();
    expect(token.length, greaterThanOrEqualTo(40));
    expect(token, isNot(contains('=')));
    expect(token, isNot(contains('/')));
    expect(token, isNot(contains('+')));
    expect(generateToken(), isNot(generateToken()));
  });

  test('loadOrCreate persists a token and stays idempotent', () async {
    final first = await tokens.loadOrCreate();
    final second = await tokens.loadOrCreate();
    expect(first, isNotEmpty);
    expect(second, first);
    expect(tokenFile.readAsStringSync().trim(), first);
  });

  test('authorize accepts the current token as a Bearer header', () async {
    final token = await tokens.loadOrCreate();
    expect(await tokens.authorize('Bearer $token'), isTrue);
    expect(await tokens.authorize(null), isFalse);
    expect(await tokens.authorize(''), isFalse);
    expect(await tokens.authorize('Basic $token'), isFalse);
    expect(await tokens.authorize('Bearer'), isFalse);
    expect(await tokens.authorize('Bearer wrong-token'), isFalse);
    expect(await tokens.authorize('Bearer $token extra'), isFalse);
  });

  test(
    'rotate invalidates the previous token and accepts the new one',
    () async {
      final before = await tokens.loadOrCreate();
      final after = await tokens.rotate();
      expect(after, isNot(before));
      expect(await tokens.authorize('Bearer $before'), isFalse);
      expect(await tokens.authorize('Bearer $after'), isTrue);
    },
  );

  test('missing or unreadable file never authorizes', () async {
    expect(await tokens.authorize('Bearer anything'), isFalse);
  });

  test('token file is written with owner-only permissions on POSIX', () async {
    if (Platform.isWindows) {
      return;
    }
    await tokens.loadOrCreate();
    final mode = (await FileStat.stat(tokenFile.path)).mode & 0x1FF;
    expect(
      mode,
      0x180,
      reason:
          'expected mode 0600, got '
          '${mode.toRadixString(8)}',
    );
  });
}

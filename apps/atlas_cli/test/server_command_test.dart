import 'dart:io';

import 'package:atlas_cli/atlas_cli.dart';
import 'package:test/test.dart';

void main() {
  test('defaults to loopback on port 8765 without flags', () {
    final options = parseServerOptions(const []);
    expect(options.address, InternetAddress.loopbackIPv4);
    expect(options.port, 8765);
    expect(options.tokenFile, isNull);
    expect(options.rotateToken, isFalse);
  });

  test('parses --listen host:port values', () {
    final explicit = parseServerOptions(const ['--listen', '0.0.0.0:9000']);
    expect(explicit.address, InternetAddress.anyIPv4);
    expect(explicit.port, 9000);

    final localhost = parseServerOptions(const ['--listen', 'localhost:1']);
    expect(localhost.address, InternetAddress.loopbackIPv4);
    expect(localhost.port, 1);
  });

  test('parses token flags', () {
    final options = parseServerOptions(const [
      '--token-file',
      '/tmp/custom-token',
    ]);
    expect(options.tokenFile, '/tmp/custom-token');
    expect(options.rotateToken, isFalse);

    final rotated = parseServerOptions(const ['--rotate-token']);
    expect(rotated.rotateToken, isTrue);
  });

  test('rejects malformed listen values and unknown flags', () {
    expect(
      () => parseServerOptions(const ['--listen', '127.0.0.1']),
      throwsFormatException,
    );
    expect(
      () => parseServerOptions(const ['--listen', '127.0.0.1:0']),
      throwsFormatException,
    );
    expect(
      () => parseServerOptions(const ['--listen', '127.0.0.1:70000']),
      throwsFormatException,
    );
    expect(
      () => parseServerOptions(const ['--listen', 'nope:80']),
      throwsFormatException,
    );
    expect(() => parseServerOptions(const ['--listen']), throwsFormatException);
    expect(() => parseServerOptions(const ['--wat']), throwsFormatException);
    expect(
      () => parseServerOptions(const ['--token-file']),
      throwsFormatException,
    );
    // --print-token was removed: the token prints on every start.
    expect(
      () => parseServerOptions(const ['--print-token']),
      throwsFormatException,
    );
  });

  test('lanReachabilityHint advertises reachable LAN addresses', () {
    const addresses = ['192.168.1.23', '100.64.0.2'];
    final onAll = lanReachabilityHint(
      bound: InternetAddress.anyIPv4,
      lanAddresses: addresses,
      port: 8765,
    );
    expect(onAll, contains('ws://192.168.1.23:8765/acp'));
    expect(onAll, contains('Phone-accessible'));
    expect(onAll, isNot(contains('restarting')));

    final concrete = lanReachabilityHint(
      bound: InternetAddress('192.168.1.23'),
      lanAddresses: addresses,
      port: 9000,
    );
    expect(concrete, contains('ws://192.168.1.23:9000/acp'));
    expect(concrete, isNot(contains('100.64.0.2')));
  });

  test('lanReachabilityHint qualifies loopback-bound candidates', () {
    final hint = lanReachabilityHint(
      bound: InternetAddress.loopbackIPv4,
      lanAddresses: const ['192.168.1.23'],
      port: 8765,
    );
    expect(hint, contains('ws://192.168.1.23:8765/acp'));
    expect(hint, contains('restarting with --listen 0.0.0.0:8765'));

    expect(
      lanReachabilityHint(
        bound: InternetAddress.loopbackIPv4,
        lanAddresses: const [],
        port: 8765,
      ),
      contains('--listen 0.0.0.0:8765'),
    );
  });
}

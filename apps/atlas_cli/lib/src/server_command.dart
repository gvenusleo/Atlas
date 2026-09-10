import 'dart:async';
import 'dart:io';

import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_ws/atlas_ws.dart';

/// Parsed `atlas server` options.
final class ServerOptions {
  /// Creates server options.
  const ServerOptions({
    this.address,
    this.port = 8765,
    this.tokenFile,
    this.rotateToken = false,
  });

  /// The interface to bind; null means loopback only.
  final InternetAddress? address;

  /// The listening TCP port.
  final int port;

  /// The token file path; defaults to `~/.atlas/remote_token`.
  final String? tokenFile;

  /// Rotates the token and exits without serving.
  final bool rotateToken;
}

/// Parses `atlas server` arguments.
///
/// Throws [FormatException] on unknown flags or malformed values.
ServerOptions parseServerOptions(List<String> args) {
  var address = InternetAddress.loopbackIPv4;
  var port = 8765;
  String? tokenFile;
  var rotateToken = false;
  for (var i = 0; i < args.length; i++) {
    switch (args[i]) {
      case '--listen':
        if (i + 1 >= args.length) {
          throw const FormatException('--listen requires host:port');
        }
        final value = args[++i];
        final (host, rawPort) = _splitHostPort(value);
        address = _parseAddress(host);
        port = _parsePort(rawPort);
      case '--token-file':
        if (i + 1 >= args.length) {
          throw const FormatException('--token-file requires a path');
        }
        tokenFile = args[++i];
      case '--rotate-token':
        rotateToken = true;
      default:
        throw FormatException('unknown server option: ${args[i]}');
    }
  }
  return ServerOptions(
    address: address,
    port: port,
    tokenFile: tokenFile,
    rotateToken: rotateToken,
  );
}

/// Runs the `atlas server` subcommand: serves the composed runtime to ACP
/// clients over WebSocket until interrupted.
Future<void> runServerCommand(
  AtlasConfig config, {
  required String home,
  required List<String> args,
}) async {
  final ServerOptions options;
  try {
    options = parseServerOptions(args);
  } on FormatException catch (error) {
    stderr.writeln('atlas server: ${error.message}');
    stderr.writeln(
      'usage: atlas server [--listen host:port] '
      '[--token-file path] [--rotate-token]',
    );
    exit(64);
  }
  final tokenFile = RemoteTokenFile(
    File(options.tokenFile ?? '$home/.atlas/remote_token'),
  );

  if (options.rotateToken) {
    final token = await tokenFile.rotate();
    stdout.writeln('Rotated the remote access token: $token');
    stdout.writeln('Keep it secret; existing connections stay open.');
    return;
  }

  // The pairing token prints on every start: users need it to connect their
  // phone, and it can be rotated at any time with --rotate-token.
  final token = await tokenFile.loadOrCreate();
  stdout.writeln('Remote access token: $token');
  stdout.writeln('Keep it secret; it grants full local agent access.');

  final runtime = composeRuntime(config);
  final server = AtlasWsServer(
    runtime: runtime,
    models: composeModels(config),
    authorize: tokenFile.authorize,
    log: (message) => stderr.writeln('[ws] $message'),
  );

  final resolvedAddress = options.address ?? InternetAddress.loopbackIPv4;
  if (resolvedAddress != InternetAddress.loopbackIPv4 &&
      resolvedAddress != InternetAddress.loopbackIPv6) {
    stderr.writeln(
      'WARNING: listening on ${resolvedAddress.address} exposes Atlas to '
      'the network. Prefer 127.0.0.1 behind Tailscale.',
    );
  }

  final httpServer = await server.start(
    address: resolvedAddress,
    port: options.port,
  );
  final bound = httpServer.address;
  stdout.writeln(
    'Atlas WebSocket server listening on '
    'ws://${bound.address}:${httpServer.port}/acp',
  );
  stdout.writeln(
    lanReachabilityHint(
      bound: bound,
      lanAddresses: await _localIPv4Addresses(),
      port: httpServer.port,
    ),
  );
  stdout.writeln('Press Ctrl+C to stop.');

  final interrupt = Completer<void>();
  for (final signal in [ProcessSignal.sigint, ProcessSignal.sigterm]) {
    unawaited(
      signal.watch().first.then((_) {
        if (!interrupt.isCompleted) {
          interrupt.complete();
        }
      }),
    );
  }
  await interrupt.future;
  await server.stop();
}

/// Returns the non-loopback IPv4 addresses of this host, best-effort.
Future<List<String>> _localIPv4Addresses() async {
  try {
    final interfaces = await NetworkInterface.list(
      type: InternetAddressType.IPv4,
      includeLoopback: false,
    );
    final seen = <String>{};
    return [
      for (final interface in interfaces)
        for (final address in interface.addresses)
          if (seen.add(address.address)) address.address,
    ];
  } on Object {
    return const [];
  }
}

/// Builds the LAN reachability hint shown when `atlas server` starts.
///
/// When [bound] covers every interface (`0.0.0.0`/`::`) or a concrete
/// non-loopback address, the listed URLs are reachable from other devices;
/// when bound to loopback they are shown as candidates that become reachable
/// only after restarting with `--listen 0.0.0.0` or via Tailscale.
///
/// Returns an empty string when nothing useful can be shown.
String lanReachabilityHint({
  required InternetAddress bound,
  required List<String> lanAddresses,
  required int port,
}) {
  if (lanAddresses.isEmpty) {
    return bound.isLoopback
        ? 'No LAN address detected; use --listen 0.0.0.0:$port (or Tailscale '
              'Serve) to let phones connect.'
        : '';
  }
  final urls = [
    for (final address in lanAddresses) '  ws://$address:$port/acp',
  ].join('\n');
  if (bound.isLoopback) {
    return 'Phone-accessible addresses (reachable after restarting with '
        '--listen 0.0.0.0:$port, or over Tailscale):\n$urls';
  }
  if (bound == InternetAddress.anyIPv4 || bound == InternetAddress.anyIPv6) {
    return 'Phone-accessible addresses on this network:\n$urls';
  }
  // Bound to one concrete interface: only it is reachable.
  final others = [
    for (final address in lanAddresses)
      if (address == bound.address) address,
  ];
  return others.isEmpty
      ? ''
      : 'Phone-accessible address on this network:\n  ws://${others.first}:$port/acp';
}

(String, String) _splitHostPort(String value) {
  final separator = value.lastIndexOf(':');
  if (separator <= 0 || separator == value.length - 1) {
    throw FormatException('--listen requires host:port (got "$value")');
  }
  return (value.substring(0, separator), value.substring(separator + 1));
}

InternetAddress _parseAddress(String host) {
  if (host == 'localhost') {
    return InternetAddress.loopbackIPv4;
  }
  return InternetAddress.tryParse(host) ??
      (throw FormatException('invalid listen address: $host'));
}

int _parsePort(String raw) {
  final port = int.tryParse(raw);
  if (port == null || port < 1 || port > 65535) {
    throw FormatException('invalid listen port: $raw');
  }
  return port;
}

import 'dart:io';

import 'package:args/args.dart';
import 'package:atlas_composition/atlas_composition.dart';
import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_ws/atlas_ws.dart';

import 'runtime_resources.dart';
import 'termination_signals.dart';

/// Parsed `atlas server` options.
final class const ServerOptions({
  /// The interface to bind; null means loopback only.
  final InternetAddress? address,

  /// The listening TCP port.
  final int port = 8765,

  /// The token file path; defaults to `~/.atlas/remote_token`.
  final String? tokenFile,

  /// Rotates the token and exits without serving.
  final bool rotateToken = false,
}) {
  /// Creates server options.
  this;

  /// Validates parsed server options before configuration or token access.
  factory fromResults(ArgResults results) {
    if (results.rest.isNotEmpty) {
      throw const FormatException('server does not take positional arguments');
    }
    final (host, rawPort) = _splitHostPort(results.option('listen')!);
    final tokenFile = results.option('token-file');
    if (tokenFile != null && tokenFile.trim().isEmpty) {
      throw const FormatException('--token-file requires a non-empty path');
    }
    return ServerOptions(
      address: _parseAddress(host),
      port: _parsePort(rawPort),
      tokenFile: tokenFile,
      rotateToken: results.flag('rotate-token'),
    );
  }
}

/// Parses `atlas server` arguments.
///
/// Throws [FormatException] on unknown flags or malformed values.
ServerOptions parseServerOptions(List<String> args) {
  final parser = ArgParser();
  addServerOptions(parser);
  return ServerOptions.fromResults(parser.parse(args));
}

/// Registers the remote server options.
void addServerOptions(ArgParser parser) {
  parser
    ..addOption(
      'listen',
      defaultsTo: '127.0.0.1:8765',
      valueHelp: 'host:port',
      help: 'Address and port to bind (loopback by default).',
    )
    ..addOption(
      'token-file',
      valueHelp: 'path',
      help: 'Token file (defaults to ~/.atlas/remote_token).',
    )
    ..addFlag(
      'rotate-token',
      negatable: false,
      help: 'Rotate the token and exit without serving.',
    );
}

/// Runs the `atlas server` subcommand: serves the composed runtime to ACP
/// clients over WebSocket until interrupted.
Future<void> runServerCommand({
  required ServerOptions options,
  required String home,
  required AtlasConfig Function() loadConfiguration,
  required StringSink out,
  required StringSink err,
}) async {
  final tokenFile = RemoteTokenFile(
    File(options.tokenFile ?? '$home/.atlas/remote_token'),
  );

  if (options.rotateToken) {
    final token = await tokenFile.rotate();
    out.writeln('Rotated the remote access token: $token');
    err.writeln('Keep it secret; existing connections stay open.');
    return;
  }

  // The pairing token prints on every start: users need it to connect their
  // phone, and it can be rotated at any time with --rotate-token.
  final config = loadConfiguration();
  final token = await tokenFile.loadOrCreate();
  out.writeln('Remote access token: $token');
  err.writeln('Keep it secret; it grants full local agent access.');

  final resources = CliRuntimeResources(config);
  final signals = TerminationSignals();
  final server = AtlasWsServer(
    runtime: resources.runtime,
    models: composeModels(config),
    authorize: tokenFile.authorize,
    log: (message) => err.writeln('[ws] $message'),
  );

  final resolvedAddress = options.address ?? InternetAddress.loopbackIPv4;
  if (resolvedAddress != InternetAddress.loopbackIPv4 &&
      resolvedAddress != InternetAddress.loopbackIPv6) {
    err.writeln(
      'WARNING: listening on ${resolvedAddress.address} exposes Atlas to '
      'the network. Prefer 127.0.0.1 behind Tailscale.',
    );
  }

  try {
    final httpServer = await server.start(
      address: resolvedAddress,
      port: options.port,
    );
    final bound = httpServer.address;
    out.writeln(
      'Atlas WebSocket server listening on '
      'ws://${bound.address}:${httpServer.port}/acp',
    );
    final hint = lanReachabilityHint(
      bound: bound,
      lanAddresses: await _localIPv4Addresses(),
      port: httpServer.port,
    );
    if (hint.isNotEmpty) out.writeln(hint);
    err.writeln('Press Ctrl+C to stop.');
    await signals.interrupted;
  } finally {
    try {
      // Closing a client normally leaves turns running. Process shutdown must
      // cancel them and drain protocol handlers before storage is closed.
      await Future.wait([server.stop(), resources.runtime.shutdown()]);
    } finally {
      await signals.close();
      await resources.close();
    }
  }
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
  final urls = [for (final address in lanAddresses) '  ws://$address:$port/acp']
      .join('\n');
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

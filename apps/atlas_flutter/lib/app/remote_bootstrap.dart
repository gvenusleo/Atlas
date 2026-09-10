import 'dart:async';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:stream_channel/stream_channel.dart';
import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'remote_connections.dart';
import 'runtime_environment.dart';

/// A connected remote session and its disconnect notification.
final class RemoteSessionHandle {
  /// Creates a handle.
  const RemoteSessionHandle({required this.environment, required this.closed});

  /// The runtime environment backed by the remote `atlas server`.
  final RuntimeEnvironment environment;

  /// Completes when the underlying WebSocket connection ends, for any reason
  /// (server shutdown, network loss, or an explicit close).
  final Future<void> closed;
}

/// Connects to a remote `atlas server` over WebSocket and wraps it in an
/// [AcpClient], exactly like the local in-process ACP bootstrap.
///
/// Each WebSocket text frame carries one ACP JSON-RPC message; binary frames
/// and frames beyond [maxFrameLength] close the connection. The model
/// catalog is discovered from a temporary session's config options and the
/// temporary session is removed again.
///
/// Throws on authentication failure, unreachable hosts, or protocol errors.
Future<RemoteSessionHandle> bootstrapRemoteConnection(
  RemoteConnectionProfile profile, {
  int maxFrameLength = 8 * 1024 * 1024,
}) async {
  final uri = Uri.tryParse(profile.wsUrl);
  if (uri == null ||
      !(uri.scheme == 'ws' || uri.scheme == 'wss') ||
      uri.host.isEmpty) {
    throw FormatException('invalid WebSocket URL: ${profile.wsUrl}');
  }
  final ws = IOWebSocketChannel.connect(
    uri,
    headers: {'Authorization': 'Bearer ${profile.token}'},
  );
  await ws.ready.timeout(const Duration(seconds: 10));
  final bridge = _bridgeWebSocket(ws, maxFrameLength);
  final client = AcpClient(ChannelTransport(bridge));
  await client.connect();
  final catalog = await _discoverCatalog(client, profile);
  return RemoteSessionHandle(
    environment: RuntimeEnvironment(
      runtime: client,
      models: catalog.isEmpty ? client.catalog : catalog,
      skills: const NoopSkillCatalog(),
      isRemote: true,
      closed: client.closed,
      onClose: () async {
        await client.close();
        await ws.sink.close();
      },
    ),
    closed: client.closed,
  );
}

/// Discovers the server's model catalog through a throwaway session.
Future<List<ModelDescriptor>> _discoverCatalog(
  AcpClient client,
  RemoteConnectionProfile profile,
) async {
  // `/` always exists on the server; the probe session is deleted right away.
  final workingDirectory = profile.workingDirectory ?? '/';
  try {
    final probe = await client.createSession(
      workingDirectory: workingDirectory,
    );
    final catalog = client.catalog;
    try {
      await client.deleteSession(probe.id);
    } on Object {
      // The probe session is harmless; leaving it behind is acceptable.
    }
    return catalog;
  } on Object {
    return client.catalog;
  }
}

/// Adapts a WebSocket channel into a String-only ACP channel.
///
/// Incoming frames must be text and are bounded by [maxFrameLength]; binary
/// frames close the connection with code 4003, oversized frames with 4009.
StreamChannel<String> _bridgeWebSocket(
  WebSocketChannel ws,
  int maxFrameLength,
) {
  final controller = StreamChannelController<String>(sync: true);
  ws.stream.listen(
    (event) {
      if (event is! String) {
        unawaited(ws.sink.close(4003, 'binary frames unsupported'));
      } else if (event.length > maxFrameLength) {
        unawaited(ws.sink.close(4009, 'frame too large'));
      } else {
        controller.local.sink.add(event);
      }
    },
    onError: (Object error, StackTrace stack) {
      controller.local.sink.addError(error, stack);
    },
    onDone: () => controller.local.sink.close(),
  );
  controller.local.stream.listen(
    (text) => ws.sink.add(text),
    onError: (Object error, StackTrace stack) {
      ws.sink.addError(error, stack);
    },
    onDone: () => ws.sink.close(),
  );
  return controller.foreign;
}

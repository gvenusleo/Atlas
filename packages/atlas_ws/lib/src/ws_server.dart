import 'dart:async';
import 'dart:io';

import 'package:atlas_acp/atlas_acp.dart';
import 'package:atlas_runtime/atlas_runtime.dart' as rt;
import 'package:shelf/shelf.dart';
import 'package:shelf/shelf_io.dart' as shelf_io;
import 'package:shelf_web_socket/shelf_web_socket.dart';
import 'package:stream_channel/stream_channel.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

/// Whether an ACP WebSocket client may connect.
///
/// Receives the `Authorization` header value (or null when absent) and must
/// return true to allow the upgrade.
typedef WsAuthorizer = Future<bool> Function(String? authorization);

/// Serves one `AcpServer` per WebSocket connection over a shared runtime.
///
/// The `/acp` endpoint upgrades WebSocket connections after a successful
/// [authorize] check. Each connection carries the ACP JSON-RPC protocol with
/// one message per text frame, exactly as `atlas acp` does over stdio.
/// Binary frames, oversized frames, and connections beyond [maxConnections]
/// are closed. Connections do not own the runtime: closing a socket leaves
/// running turns untouched so they finish and persist.
final class AtlasWsServer {
  /// Creates a WebSocket ACP server over [runtime].
  ///
  /// [models] is the configured model catalog offered through session
  /// `configOptions`, as with the stdio `AcpServer`. [authorize] guards the
  /// HTTP upgrade; [log] receives lifecycle events (never token material).
  AtlasWsServer({
    required this.runtime,
    this.models = const [],
    required this.authorize,
    this.maxConnections = 4,
    this.maxFrameLength = 8 * 1024 * 1024,
    this.pingInterval = const Duration(seconds: 30),
    this.log,
  });

  /// The shared runtime serving every connection.
  final rt.AgentEngine runtime;

  /// The configured model catalog, in display priority order.
  final List<rt.ModelDescriptor> models;

  /// Guards the HTTP upgrade of every connection.
  final WsAuthorizer authorize;

  /// Maximum simultaneous WebSocket connections.
  final int maxConnections;

  /// Maximum accepted text frame length in characters.
  final int maxFrameLength;

  /// How often the server pings clients to detect dead connections.
  final Duration pingInterval;

  /// Receives lifecycle events; never receives token material.
  final void Function(String message)? log;

  final _connections = <WebSocketChannel>{};
  var _active = 0;

  /// Number of currently connected clients.
  int get activeConnections => _active;

  HttpServer? _listener;

  late final Handler _upgradeHandler = webSocketHandler(
    _onConnection,
    pingInterval: pingInterval,
  );

  /// Starts listening on [address]:[port] and returns the HTTP server.
  ///
  /// Defaults to loopback so the endpoint is not exposed to the network
  /// unless the caller opts in.
  Future<HttpServer> start({Object? address, int port = 8765}) async {
    final server = await shelf_io.serve(
      _handler,
      address ?? InternetAddress.loopbackIPv4,
      port,
    );
    _listener = server;
    return server;
  }

  /// Closes the listener and every live connection.
  Future<void> stop() async {
    await _listener?.close(force: true);
    final live = _connections.toList();
    for (final connection in live) {
      // The underlying web_socket implementation only accepts close code
      // 1000 or the 3000-4999 range.
      await connection.sink.close(4001, 'server shutting down');
    }
  }

  FutureOr<Response> _handler(Request request) {
    if (request.url.path != 'acp' || request.method != 'GET') {
      return Response.notFound('Not found');
    }
    if (!_isUpgradeRequest(request)) {
      // Not an upgrade: let the upgrade handler answer with its own 400/404.
      return _upgradeHandler(request);
    }
    return _authorized(request);
  }

  Future<Response> _authorized(Request request) async {
    if (!await authorize(request.headers['authorization'])) {
      return Response(401, body: 'Unauthorized');
    }
    return _upgradeHandler(request);
  }

  void _onConnection(WebSocketChannel channel, String? protocol) {
    if (_active >= maxConnections) {
      unawaited(channel.sink.close(4013, 'too many connections'));
      return;
    }
    _active++;
    _connections.add(channel);
    log?.call('client connected ($_active/$maxConnections)');

    // Bridge the channel into a String-only StreamChannel for AcpServer:
    // enforce the text-frame policy here and reject binary payloads.
    final incoming = StreamController<String>();
    channel.stream.listen(
      (event) {
        if (event is! String) {
          _reject(incoming, channel, 4003, 'binary frames unsupported');
        } else if (event.length > maxFrameLength) {
          _reject(incoming, channel, 4009, 'frame too large');
        } else {
          incoming.add(event);
        }
      },
      onError: (_) => incoming.close(),
      onDone: () {
        if (!incoming.isClosed) {
          incoming.close();
        }
      },
    );
    final outbound = StreamController<String>();
    outbound.stream.listen((text) {
      try {
        channel.sink.add(text);
      } on StateError {
        // The connection closed between frames; the sender observes it on
        // its own side through the closed incoming stream.
      }
    });
    final bridge = StreamChannel<String>(incoming.stream, outbound.sink);
    unawaited(
      AcpServer(runtime, models: models).serveChannel(bridge).whenComplete(
        () async {
          _active--;
          _connections.remove(channel);
          log?.call('client disconnected ($_active/$maxConnections)');
          if (!incoming.isClosed) {
            await incoming.close();
          }
          if (!outbound.isClosed) {
            await outbound.close();
          }
          await channel.sink.close();
        },
      ),
    );
  }

  /// Rejects [channel] and ends the ACP session immediately.
  ///
  /// Closing [incoming] terminates the serveChannel future so the connection
  /// slot frees without waiting for the peer's close handshake.
  void _reject(
    StreamController<String> incoming,
    WebSocketChannel channel,
    int code,
    String reason,
  ) {
    unawaited(channel.sink.close(code, reason));
    unawaited(incoming.close());
  }
}

bool _isUpgradeRequest(Request request) {
  final connection = request.headers['Connection']?.toLowerCase();
  if (connection == null || !connection.split(',').contains('upgrade')) {
    return false;
  }
  return request.headers['Upgrade']?.toLowerCase() == 'websocket';
}

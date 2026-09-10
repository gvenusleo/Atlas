import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_ws/atlas_ws.dart';
import 'package:test/test.dart';
import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

void main() {
  late Directory temp;
  late RemoteTokenFile tokens;
  late AtlasWsServer server;
  late HttpServer httpServer;

  setUp(() async {
    temp = await Directory.systemTemp.createTemp('atlas_ws_server_test');
    tokens = RemoteTokenFile(File('${temp.path}/remote_token'));
    await tokens.loadOrCreate();
    server = AtlasWsServer(
      runtime: _testRuntime(),
      authorize: tokens.authorize,
      log: (_) {},
    );
    httpServer = await server.start(port: 0);
  });

  tearDown(() async {
    await server.stop();
    await httpServer.close(force: true);
    await temp.delete(recursive: true);
  });

  /// Connects to [http] (defaults to the main test server) and waits for the
  /// upgrade; authentication failures surface as an error from [ready].
  Future<_TestClient> connect({
    HttpServer? http,
    String? token,
    String path = 'acp',
  }) async {
    final target = http ?? httpServer;
    final channel = IOWebSocketChannel.connect(
      Uri.parse('ws://127.0.0.1:${target.port}/$path'),
      headers: token == null ? null : {'Authorization': 'Bearer $token'},
    );
    await channel.ready;
    return _TestClient(channel);
  }

  test('upgrade without a token is rejected', () async {
    await expectLater(connect(), throwsA(isA<Exception>()));
    expect(server.activeConnections, 0);
  });

  test('upgrade with a wrong token is rejected', () async {
    await expectLater(connect(token: 'Bearer nope'), throwsA(isA<Exception>()));
  });

  test('plain GET and unknown paths return 404', () async {
    final client = HttpClient();
    for (final path in ['acp', 'other']) {
      final request = await client.get('127.0.0.1', httpServer.port, '/$path');
      final response = await request.close();
      expect(response.statusCode, 404, reason: 'path /$path');
      await response.drain<void>();
    }
    client.close();
  });

  test('initializes and creates a session over a valid connection', () async {
    final token = await tokens.loadOrCreate();
    final ws = await connect(token: token);
    addTearDown(ws.close);

    final initialized = await ws.sendRpc(1, 'initialize', {
      'protocolVersion': 1,
    });
    if (initialized.containsKey('error')) {
      fail('initialize failed: ${initialized['error']}');
    }
    final result = initialized['result'] as Map<String, Object?>;
    expect(result['protocolVersion'], 1);
    expect(result['agentCapabilities'], isA<Map<String, Object?>>());

    final created = await ws.sendRpc(2, 'session/new', {'cwd': '/tmp'});
    final createdResult = created['result'] as Map<String, Object?>;
    expect(createdResult['sessionId'], isA<String>());
  });

  test(
    'rotating the token rejects new connections with the old token',
    () async {
      final before = await tokens.loadOrCreate();
      await tokens.rotate();
      await expectLater(connect(token: before), throwsA(isA<Exception>()));
      final after = await tokens.rotate();
      final ws = await connect(token: after);
      addTearDown(ws.close);
      expect(server.activeConnections, 1);
    },
  );

  test('connections beyond the limit are closed', () async {
    final limited = AtlasWsServer(
      runtime: _testRuntime(),
      authorize: tokens.authorize,
      maxConnections: 1,
      log: (_) {},
    );
    final http = await limited.start(port: 0);
    addTearDown(limited.stop);
    addTearDown(http.close);
    final token = await tokens.loadOrCreate();
    final first = await connect(http: http, token: token);
    addTearDown(first.close);
    final second = await connect(http: http, token: token);
    // The second connection is closed by the server (code 4013).
    final closeCode = await second.doneCloseCode();
    expect(closeCode, 4013);
  });

  test('binary frames close the connection', () async {
    final token = await tokens.loadOrCreate();
    final raw = await WebSocket.connect(
      'ws://127.0.0.1:${httpServer.port}/acp',
      headers: {'Authorization': 'Bearer $token'},
    );
    addTearDown(() => raw.close());
    raw.add([1, 2, 3]); // A binary frame.
    // Consume inbound events (single-subscription socket): the server's
    // close handshake only completes when the stream is being listened to.
    await raw.drain<void>().timeout(const Duration(seconds: 5));
    expect(raw.closeCode, 4003);
  });

  test('oversized text frames close the connection', () async {
    final tight = AtlasWsServer(
      runtime: _testRuntime(),
      authorize: tokens.authorize,
      maxFrameLength: 1024,
      log: (_) {},
    );
    final http = await tight.start(port: 0);
    addTearDown(tight.stop);
    addTearDown(http.close);
    final token = await tokens.loadOrCreate();
    final ws = await connect(http: http, token: token);
    addTearDown(ws.close);
    ws.sink.add('x' * 2048);
    final closeCode = await ws.doneCloseCode();
    expect(closeCode, 4009);
  });

  test('closing the client releases the connection slot', () async {
    final token = await tokens.loadOrCreate();
    final ws = await connect(token: token);
    expect(server.activeConnections, 1);
    await ws.close();
    // The server notices the close frame asynchronously.
    await Future<void>.delayed(const Duration(milliseconds: 300));
    expect(server.activeConnections, 0);
  });
}

/// A single-listener JSON-RPC client over one WebSocket channel.
final class _TestClient {
  _TestClient(this._channel) {
    final done = Completer<void>();
    _subscription = _channel.stream.listen(
      (frame) {
        final message = jsonDecode(frame as String) as Map<String, Object?>;
        final id = message['id'];
        if (id != null) {
          _pending.remove(id)?.complete(message);
        }
      },
      onDone: done.complete,
      onError: (Object error, StackTrace stack) {
        done.completeError(error, stack);
      },
    );
    _done = done.future;
  }

  final WebSocketChannel _channel;
  final _pending = <Object?, Completer<Map<String, Object?>>>{};
  late final StreamSubscription<dynamic> _subscription;
  late final Future<void> _done;

  /// The underlying channel sink, for raw sends.
  WebSocketSink get sink => _channel.sink;

  /// Sends a JSON-RPC request and resolves with its result or error.
  Future<Map<String, Object?>> sendRpc(
    int id,
    String method, [
    Map<String, Object?> params = const {},
  ]) {
    final completer = Completer<Map<String, Object?>>();
    _pending[id] = completer;
    sink.add(
      jsonEncode({
        'jsonrpc': '2.0',
        'id': id,
        'method': method,
        if (params.isNotEmpty) 'params': params,
      }),
    );
    return completer.future.timeout(const Duration(seconds: 5));
  }

  /// The close code observed when the server closes this connection, or null
  /// when the stream ends without a close frame.
  Future<int?> doneCloseCode() async {
    await _done.timeout(const Duration(seconds: 5));
    return _channel.closeCode;
  }

  /// Closes the connection.
  Future<void> close() async {
    await _subscription.cancel();
    await _channel.sink.close();
  }
}

/// Builds a runtime whose store only supports session creation: the WebSocket
/// integration tests exercise transport and connection semantics, and the ACP
/// protocol behavior itself is covered by atlas_acp tests.
AgentRuntime _testRuntime() => AgentRuntime(
  store: _SessionStore(),
  provider: _UnusedProvider(),
  tools: _NoTools(),
  ids: _TestIds(),
  defaultModel: ModelRef(
    providerId: ProviderId('test'),
    modelId: ModelId('model'),
  ),
);

final class _TestIds implements IdGenerator {
  var _session = 0;

  @override
  SessionId sessionId() => SessionId('session-${++_session}');

  @override
  TurnId turnId() => TurnId('turn-${++_session}');

  @override
  TimelineItemId timelineItemId() => TimelineItemId('item-${++_session}');
}

final class _SessionStore implements SessionStore {
  final _sessions = <String, Session>{};

  @override
  Future<void> createSession(Session session) async {
    _sessions[session.id.value] = session;
  }

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Future<SessionPage> listSessions(SessionQuery query) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Future<void> beginTurn(BeginTurn operation) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  ) => throw UnsupportedError('not used by transport tests');

  @override
  Future<void> appendToolResult(SessionId sessionId, ToolResultItem item) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Future<void> finishTurn(SessionId sessionId, Turn turn) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  ) => throw UnsupportedError('not used by transport tests');

  @override
  Future<void> deleteSession(SessionId sessionId) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Future<void> renameSession(SessionId sessionId, String title) =>
      throw UnsupportedError('not used by transport tests');
}

final class _UnusedProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) =>
      throw UnsupportedError('not used by transport tests');

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) =>
      throw UnsupportedError('not used by transport tests');
}

final class _NoTools implements ToolRegistry {
  @override
  List<ToolDescriptor> get descriptors => const [];

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) =>
      throw UnsupportedError('not used by transport tests');
}

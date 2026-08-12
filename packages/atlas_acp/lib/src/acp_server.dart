import 'dart:async';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:json_rpc_2/json_rpc_2.dart';
import 'package:stream_channel/stream_channel.dart';

import 'acp_types.dart';
import 'stdio_transport.dart';
import 'update_mapper.dart';

/// The ACP agent adapter for Atlas.
///
/// Exposes the runtime to ACP clients (editors such as Zed) over NDJSON
/// stdio. Atlas owns the JSON-RPC lifecycle here; the adapter maps protocol
/// methods to runtime calls and streams runtime events back as
/// `session/update` notifications.
final class AcpServer {
  /// Creates an ACP server over [runtime].
  AcpServer(this.runtime);

  /// The runtime serving ACP sessions.
  final AgentRuntime runtime;

  final _activeTurns = <String, CancellationToken>{};
  StreamSink<String>? _output;

  /// Serves ACP over [input] and [output], completing when the connection
  /// closes.
  ///
  /// Defaults to the process stdin/stdout. Logging must go to stderr; stdout
  /// carries only protocol messages.
  Future<void> serve({Stream<List<int>>? input, IOSink? output}) async {
    await serveChannel(
      ndjsonChannel(input ?? stdin, StdoutLineSink(output ?? stdout)),
    );
  }

  /// Serves over an existing string channel, exposed for tests.
  Future<void> serveChannel(StreamChannel<String> channel) async {
    _output = channel.sink;
    final server = Server(
      channel,
      onUnhandledError: (error, stackTrace) {
        stderr.writeln('atlas_acp: unhandled error: $error');
      },
    );
    _registerMethods(server);
    await server.listen();
  }

  void _registerMethods(Server server) {
    server
      ..registerMethod('initialize', _initialize)
      ..registerMethod('session/new', _newSession)
      ..registerMethod('session/load', _loadSession)
      ..registerMethod('session/resume', _resumeSession)
      ..registerMethod('session/prompt', _prompt)
      ..registerMethod('session/list', _listSessions)
      ..registerMethod('session/close', _closeSession)
      // A notification is a request without an id; json_rpc_2 dispatches it
      // through the same registration path.
      ..registerMethod('session/cancel', _cancelPrompt);
  }

  Future<JsonObject> _initialize(Parameters params) async => initializeResult();

  Future<JsonObject> _newSession(Parameters params) async {
    final cwd = params['cwd'].asString;
    final additional = params['additionalDirectories'].valueOr(null);
    final additionalDirectories = switch (additional) {
      null => const <String>[],
      final List<Object?> list => [
        for (final entry in list)
          if (entry is String) entry,
      ],
      _ => throw RpcException.invalidParams(
        'additionalDirectories must be an array',
      ),
    };
    final session = await runtime.createSession(
      workingDirectory: cwd,
      additionalDirectories: additionalDirectories,
    );
    return {'sessionId': session.id.value};
  }

  Future<Object?> _loadSession(Parameters params) async {
    final snapshot = await _load(params);
    for (final update in replayTimeline(snapshot.timeline)) {
      _sendUpdate(update);
    }
    return null;
  }

  Future<JsonObject> _resumeSession(Parameters params) async {
    await _load(params);
    return <String, Object?>{};
  }

  Future<SessionSnapshot> _load(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    try {
      return await runtime.loadSession(SessionId(sessionId));
    } on SessionNotFoundException {
      throw RpcException.invalidParams('session not found: $sessionId');
    }
  }

  Future<JsonObject> _prompt(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    final session = SessionId(sessionId);
    if (_activeTurns.containsKey(sessionId)) {
      throw RpcException(
        -32603,
        'session already has an active turn: $sessionId',
      );
    }
    final text = _promptText(params['prompt'].asList);
    final cancellation = CancellationToken();
    _activeTurns[sessionId] = cancellation;
    final mapper = TurnUpdateMapper(session);
    try {
      String? stopReason;
      await for (final event in runtime.run(
        TurnRequest(
          sessionId: session,
          content: [TextContent(text)],
          cancellation: cancellation,
        ),
      )) {
        for (final update in mapper.map(event)) {
          _sendUpdate(update);
        }
        if (event case TurnFinished(:final outcome)) {
          stopReason = switch (outcome.status) {
            TurnStatus.completed => 'end_turn',
            TurnStatus.cancelled => 'cancelled',
            TurnStatus.failed => throw RpcException(
              -32603,
              'turn failed (${outcome.failure?.code ?? 'unknown'})',
            ),
            TurnStatus.running => null,
          };
        }
      }
      return {'stopReason': stopReason};
    } on RpcException {
      rethrow;
    } catch (error) {
      throw RpcException(-32603, 'turn failed (${error.runtimeType})');
    } finally {
      _activeTurns.remove(sessionId);
    }
  }

  void _cancelPrompt(Parameters params) {
    final sessionId = params['sessionId'].valueOr(null) as String?;
    if (sessionId == null) {
      return;
    }
    _activeTurns[sessionId]?.cancel();
  }

  Future<JsonObject> _listSessions(Parameters params) async {
    final cwd = params['cwd'].valueOr(null) as String?;
    final cursor = params['cursor'].valueOr(null) as String?;
    final page = await runtime.listSessions(
      workingDirectory: cwd,
      cursor: cursor,
    );
    return {
      'sessions': [
        for (final session in page.items)
          {
            'sessionId': session.id.value,
            'cwd': session.workingDirectory,
            if (session.title.isNotEmpty) 'title': session.title,
            'updatedAt': session.updatedAt.toUtc().toIso8601String(),
          },
      ],
      if (page.nextCursor != null) 'nextCursor': page.nextCursor,
    };
  }

  Future<JsonObject> _closeSession(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    _activeTurns.remove(sessionId)?.cancel();
    return <String, Object?>{};
  }

  /// Extracts the single text payload from an ACP prompt array.
  ///
  /// Atlas advertises only baseline text prompt capabilities, so non-text
  /// content blocks are rejected.
  static String _promptText(List<Object?> blocks) {
    if (blocks.isEmpty) {
      throw RpcException.invalidParams('prompt must not be empty');
    }
    final texts = <String>[];
    for (final block in blocks) {
      if (block is! Map) {
        throw RpcException.invalidParams('prompt blocks must be objects');
      }
      if (block['type'] != 'text') {
        throw RpcException.invalidParams(
          'unsupported content block type: ${block['type']}',
        );
      }
      final text = block['text'];
      if (text is! String) {
        throw RpcException.invalidParams('text block must contain text');
      }
      if (text.isNotEmpty) {
        texts.add(text);
      }
    }
    if (texts.isEmpty) {
      throw RpcException.invalidParams('prompt must contain text');
    }
    return texts.join('\n\n');
  }

  void _sendUpdate(SessionUpdate update) {
    _output?.add(update.toJsonString());
  }
}

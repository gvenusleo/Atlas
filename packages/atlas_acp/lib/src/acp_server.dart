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
  bool _debug = false;

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
    // ACP stdout carries only protocol messages; all diagnostics go to
    // stderr. ACP_DEBUG follows the ecosystem convention (Gemini CLI,
    // acpcli) and logs request/response summaries without payload text.
    _debug = Platform.environment['ACP_DEBUG'] == '1';
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
      ..registerMethod('initialize', _requestLog(_initialize))
      ..registerMethod('session/new', _requestLog(_newSession))
      ..registerMethod('session/load', _requestLog(_loadSession))
      ..registerMethod('session/resume', _requestLog(_resumeSession))
      ..registerMethod('session/prompt', _requestLog(_prompt))
      ..registerMethod('session/list', _requestLog(_listSessions))
      ..registerMethod('session/close', _requestLog(_closeSession))
      // A notification is a request without an id; json_rpc_2 dispatches it
      // through the same registration path.
      ..registerMethod('session/cancel', _requestLog(_cancelPrompt));
  }

  /// Wraps [handler] with request/response logging when ACP_DEBUG is set.
  FutureOr<Object?> Function(Parameters) _requestLog(
    FutureOr<Object?> Function(Parameters) handler,
  ) {
    return (params) async {
      final sessionId = params['sessionId'].valueOr(null);
      if (_debug) {
        stderr.writeln(
          'atlas_acp: << ${params.method}'
          '${sessionId == null ? '' : ' session=$sessionId'}',
        );
      }
      try {
        final result = await handler(params);
        if (_debug) {
          stderr.writeln('atlas_acp: >> ${params.method}: ok');
        }
        return result;
      } catch (error) {
        if (_debug) {
          stderr.writeln(
            'atlas_acp: >> ${params.method}: '
            '${error is RpcException ? 'error ${error.code} ${error.message}' : error.runtimeType}',
          );
        }
        if (error is RpcException) {
          rethrow;
        }
        // Never let raw exceptions reach json_rpc_2's default serializer,
        // which embeds the full stack trace in the error data.
        throw RpcException(-32603, 'internal error (${error.runtimeType})');
      }
    };
  }

  Future<JsonObject> _initialize(Parameters params) async => initializeResult();

  Future<JsonObject> _newSession(Parameters params) async {
    final cwd = _absolutePath(params['cwd'].asString, 'cwd');
    final additional = params['additionalDirectories'].valueOr(null);
    final additionalDirectories = switch (additional) {
      null => const <String>[],
      final List<Object?> list => [
        for (final entry in list)
          if (entry is String)
            _absolutePath(entry, 'additionalDirectories')
          else
            throw RpcException.invalidParams(
              'additionalDirectories must contain only strings',
            ),
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
    _absolutePath(params['cwd'].asString, 'cwd');
    try {
      return await runtime.loadSession(SessionId(sessionId));
    } on SessionNotFoundException {
      throw RpcException.invalidParams('session not found: $sessionId');
    }
  }

  Future<JsonObject> _prompt(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    if (sessionId.isEmpty) {
      throw RpcException.invalidParams('sessionId must not be empty');
    }
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
      String? failureCode;
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
          switch (outcome.status) {
            case TurnStatus.completed:
              stopReason = 'end_turn';
            case TurnStatus.cancelled:
              stopReason = 'cancelled';
            case TurnStatus.failed:
              failureCode = outcome.failure?.code;
            case TurnStatus.running:
              break;
          }
        }
      }
      // Consume the whole stream (including trailing compaction events)
      // before reporting the failure so the runtime turn settles.
      if (failureCode != null) {
        throw RpcException(-32603, 'turn failed ($failureCode)');
      }
      if (stopReason == null) {
        throw RpcException(-32603, 'turn ended without a stop reason');
      }
      return {'stopReason': stopReason};
    } on SessionNotFoundException catch (error) {
      throw RpcException.invalidParams('session not found: ${error.sessionId}');
    } on RpcException {
      rethrow;
    } catch (error) {
      throw RpcException(-32603, 'turn failed (${error.runtimeType})');
    } finally {
      _activeTurns.remove(sessionId);
    }
  }

  void _cancelPrompt(Parameters params) {
    final sessionId = _optionalString(params, 'sessionId');
    if (sessionId == null) {
      return;
    }
    _activeTurns[sessionId]?.cancel();
  }

  Future<JsonObject> _listSessions(Parameters params) async {
    final cwd = _optionalString(params, 'cwd');
    if (cwd != null) {
      _absolutePath(cwd, 'cwd');
    }
    final cursor = _optionalString(params, 'cursor');
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

  /// Extracts the text payload from an ACP prompt array.
  ///
  /// Atlas advertises only baseline text prompt capabilities, so non-text
  /// content blocks are rejected. `resource_link` blocks are a baseline
  /// content type and are accepted but ignored: Atlas tools access the local
  /// filesystem directly and do not need client-provided resource contents.
  static String _promptText(List<Object?> blocks) {
    if (blocks.isEmpty) {
      throw RpcException.invalidParams('prompt must not be empty');
    }
    final texts = <String>[];
    for (final block in blocks) {
      if (block is! Map) {
        throw RpcException.invalidParams('prompt blocks must be objects');
      }
      final type = block['type'];
      if (type == 'resource_link') {
        continue;
      }
      if (type != 'text') {
        throw RpcException.invalidParams(
          'unsupported content block type: $type',
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

  /// Returns the optional string parameter, rejecting non-string values with
  /// an invalid-params error.
  static String? _optionalString(Parameters params, String key) {
    final value = params[key].valueOr(null);
    if (value == null) {
      return null;
    }
    if (value is String) {
      return value;
    }
    throw RpcException.invalidParams('$key must be a string');
  }

  /// Validates that [path] is absolute, as required by ACP, and returns it.
  static String _absolutePath(String path, String field) {
    // Accepts POSIX and Windows drive-letter roots.
    final isAbsolute =
        path.startsWith('/') || RegExp(r'^[A-Za-z]:[\\/]').hasMatch(path);
    if (!isAbsolute) {
      throw RpcException.invalidParams('$field must be an absolute path');
    }
    return path;
  }

  void _sendUpdate(SessionUpdate update) {
    if (_debug) {
      final kind = update.update['sessionUpdate'];
      stderr.writeln('atlas_acp: ~ session=${update.sessionId} update=$kind');
    }
    _output?.add(update.toJsonString());
  }
}

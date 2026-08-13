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
  AcpServer(this.runtime)
    : _toolTitles = {
        for (final tool in runtime.tools.descriptors)
          if (tool.description.trim().isNotEmpty)
            tool.name: tool.description.trim(),
      };

  /// The runtime serving ACP sessions.
  final AgentRuntime runtime;

  /// Tool names mapped to their model-facing descriptions, used as the
  /// human-readable `tool_call` title required by ACP.
  final Map<String, String> _toolTitles;

  /// Last title reported to the client per session, used to emit
  /// `session_info_update` when the runtime auto-generates a title.
  final _titles = <String, String>{};

  /// Cancellation tokens of pending turns per session. The runtime serializes
  /// turns per session, so several prompts may be queued behind one active
  /// turn; cancelling must reach all of them.
  final _activeTurns = <String, List<CancellationToken>>{};

  /// Delays connection close until in-flight requests have responded, so EOF
  /// never drops a pending response.
  final _inFlight = _InFlightGate();

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
      StreamChannel<String>(_inFlight.wrap(channel.stream), channel.sink),
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
      ..registerMethod('session/delete', _requestLog(_deleteSession))
      // A notification is a request without an id; json_rpc_2 dispatches it
      // through the same registration path.
      ..registerMethod('session/cancel', _requestLog(_cancelPrompt));
  }

  /// Wraps [handler] with in-flight tracking and request/response logging
  /// when ACP_DEBUG is set.
  FutureOr<Object?> Function(Parameters) _requestLog(
    FutureOr<Object?> Function(Parameters) handler,
  ) {
    return (params) {
      final sessionId = params['sessionId'].valueOr(null);
      if (_debug) {
        stderr.writeln(
          'atlas_acp: << ${params.method}'
          '${sessionId == null ? '' : ' session=$sessionId'}',
        );
      }
      return _inFlight.run(() async {
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
      });
    };
  }

  Future<JsonObject> _initialize(Parameters params) async => initializeResult();

  Future<JsonObject> _newSession(Parameters params) async {
    _rejectMcpServers(params);
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
    _titles[session.id.value] = session.title;
    return {'sessionId': session.id.value};
  }

  Future<Object?> _loadSession(Parameters params) async {
    final snapshot = await _load(params);
    _titles[snapshot.session.id.value] = snapshot.session.title;
    for (final update in replayTimeline(snapshot.timeline, _toolTitles)) {
      _sendUpdate(update);
    }
    return null;
  }

  Future<JsonObject> _resumeSession(Parameters params) async {
    final snapshot = await _load(params);
    _titles[snapshot.session.id.value] = snapshot.session.title;
    return <String, Object?>{};
  }

  Future<SessionSnapshot> _load(Parameters params) async {
    _rejectMcpServers(params);
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
    final content = _promptContent(params['prompt'].asList);
    final cancellation = CancellationToken();
    (_activeTurns[sessionId] ??= <CancellationToken>[]).add(cancellation);
    // The runtime serializes turns per session, so a prompt sent while
    // another turn is running waits for it instead of failing.
    final mapper = TurnUpdateMapper(session, _toolTitles);
    try {
      String? stopReason;
      String? failureCode;
      int? usedTokens;
      await for (final event in runtime.run(
        TurnRequest(
          sessionId: session,
          content: content,
          cancellation: cancellation,
        ),
      )) {
        for (final update in mapper.map(event)) {
          _sendUpdate(update);
        }
        if (event case TurnFinished(:final outcome)) {
          switch (outcome.status) {
            case TurnStatus.completed:
              stopReason = outcome.stopReason == StopReason.maxTokens
                  ? 'max_tokens'
                  : 'end_turn';
              usedTokens = outcome.usage.totalTokens;
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
      // Title and usage notifications are best-effort: a failure here must
      // not fail a turn that already completed.
      await _bestEffort(() => _notifyTitleChange(session));
      await _bestEffort(() => _sendUsageUpdate(session, usedTokens));
      return {'stopReason': stopReason};
    } on SessionNotFoundException catch (error) {
      throw RpcException.invalidParams('session not found: ${error.sessionId}');
    } on RpcException {
      rethrow;
    } catch (error) {
      throw RpcException(-32603, 'turn failed (${error.runtimeType})');
    } finally {
      final pending = _activeTurns[sessionId];
      if (pending != null) {
        pending.remove(cancellation);
        if (pending.isEmpty) {
          _activeTurns.remove(sessionId);
        }
      }
    }
  }

  void _cancelPrompt(Parameters params) {
    final sessionId = _optionalString(params, 'sessionId');
    if (sessionId == null) {
      return;
    }
    for (final token
        in _activeTurns[sessionId] ?? const <CancellationToken>[]) {
      token.cancel();
    }
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
    // Record listed titles so a later prompt does not re-send a title the
    // client already saw in the list response.
    for (final item in page.items) {
      _titles[item.id.value] = item.title;
    }
    return {
      'sessions': [
        for (final session in page.items)
          {
            'sessionId': session.id.value,
            'cwd': session.workingDirectory,
            if (session.additionalDirectories.isNotEmpty)
              'additionalDirectories': session.additionalDirectories,
            if (session.title.isNotEmpty) 'title': session.title,
            'updatedAt': session.updatedAt.toUtc().toIso8601String(),
          },
      ],
      if (page.nextCursor != null) 'nextCursor': page.nextCursor,
    };
  }

  Future<JsonObject> _closeSession(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    for (final token
        in _activeTurns[sessionId] ?? const <CancellationToken>[]) {
      token.cancel();
    }
    return <String, Object?>{};
  }

  Future<JsonObject> _deleteSession(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    for (final token
        in _activeTurns[sessionId] ?? const <CancellationToken>[]) {
      token.cancel();
    }
    try {
      await runtime.deleteSession(SessionId(sessionId));
    } on SessionNotFoundException {
      // Deleting an unknown or already-deleted session succeeds silently.
    }
    _titles.remove(sessionId);
    return <String, Object?>{};
  }

  /// Emits `session_info_update` when the runtime has auto-generated a title
  /// (from the first user message) that the client has not seen yet.
  Future<void> _notifyTitleChange(SessionId session) async {
    try {
      final snapshot = await runtime.loadSession(session);
      final title = snapshot.session.title;
      if (title.isNotEmpty && title != _titles[session.value]) {
        _titles[session.value] = title;
        _sendUpdate(sessionInfoUpdate(session, title: title));
      }
    } on SessionNotFoundException {
      // The session was deleted mid-turn; there is nothing to update.
    }
  }

  /// Runs [action], swallowing failures: optional ACP notifications must
  /// never turn a completed turn into an error response.
  Future<void> _bestEffort(Future<void> Function() action) async {
    try {
      await action();
    } catch (_) {
      // Best-effort by design.
    }
  }

  int? _contextSize;

  /// The cached context window of the default model, or 0 when unknown.
  ///
  /// Only positive values are cached, so a transient describe failure is
  /// retried on the next turn instead of disabling usage updates forever.
  Future<int> _contextWindow() async {
    final cached = _contextSize;
    if (cached != null && cached > 0) {
      return cached;
    }
    final size = await runtime.contextWindowSize();
    if (size > 0) {
      _contextSize = size;
    }
    return size;
  }

  /// Emits a `usage_update` after a completed turn when usage is known.
  Future<void> _sendUsageUpdate(SessionId session, int? usedTokens) async {
    if (usedTokens == null || usedTokens <= 0) {
      return;
    }
    final size = await _contextWindow();
    if (size <= 0) {
      return;
    }
    _sendUpdate(usageUpdate(session, used: usedTokens, size: size));
  }

  /// Converts an ACP prompt array into runtime content parts.
  ///
  /// Atlas advertises text + image prompt capabilities, so those blocks are
  /// accepted; `resource` blocks are accepted once embeddedContext support
  /// advertises them. `resource_link` blocks are a baseline content type and
  /// are ignored: Atlas tools access the local filesystem directly and do not
  /// need client-provided resource contents.
  static List<ContentPart> _promptContent(List<Object?> blocks) {
    if (blocks.isEmpty) {
      throw RpcException.invalidParams('prompt must not be empty');
    }
    final parts = <ContentPart>[];
    for (final block in blocks) {
      if (block is! Map) {
        throw RpcException.invalidParams('prompt blocks must be objects');
      }
      switch (block['type']) {
        case 'text':
          final text = block['text'];
          if (text is! String) {
            throw RpcException.invalidParams('text block must contain text');
          }
          if (text.isNotEmpty) {
            parts.add(TextContent(text));
          }
        case 'image':
          parts.add(_imageContent(block));
        case 'resource':
          parts.add(_resourceContent(block));
        case 'resource_link':
          break;
        default:
          throw RpcException.invalidParams(
            'unsupported content block type: ${block['type']}',
          );
      }
    }
    if (parts.isEmpty) {
      throw RpcException.invalidParams(
        'prompt must contain text or image content',
      );
    }
    return parts;
  }

  /// Builds an image part from an ACP image block (base64 data).
  static ImageContent _imageContent(Map<Object?, Object?> block) {
    final data = block['data'];
    final mimeType = block['mimeType'];
    if (data is! String || data.isEmpty) {
      throw RpcException.invalidParams('image block must contain data');
    }
    if (mimeType is! String || mimeType.isEmpty) {
      throw RpcException.invalidParams('image block must contain mimeType');
    }
    return ImageContent(
      source: 'data:$mimeType;base64,$data',
      mimeType: mimeType,
    );
  }

  /// Builds a resource part from an ACP resource block (text payload).
  static ResourceContent _resourceContent(Map<Object?, Object?> block) {
    final resource = block['resource'];
    if (resource is! Map<Object?, Object?>) {
      throw RpcException.invalidParams(
        'resource block must contain a resource object',
      );
    }
    final uri = resource['uri'];
    final text = resource['text'];
    if (uri is! String || uri.isEmpty) {
      throw RpcException.invalidParams('resource must contain a uri');
    }
    if (text is! String) {
      throw RpcException.invalidParams('resource text must be a string');
    }
    final mimeType = resource['mimeType'];
    return ResourceContent(
      uri: uri,
      mimeType: mimeType is String && mimeType.isNotEmpty ? mimeType : null,
      text: text,
    );
  }

  /// Rejects a non-empty `mcpServers` parameter: Atlas does not implement
  /// MCP server connections (a MUST-level ACP stdio capability), so a client
  /// asking for them gets an explicit error instead of silent failure.
  static void _rejectMcpServers(Parameters params) {
    final value = params['mcpServers'].valueOr(null);
    if (value == null) {
      return;
    }
    if (value is! List) {
      throw RpcException.invalidParams('mcpServers must be an array');
    }
    if (value.isNotEmpty) {
      throw RpcException.invalidParams(
        'mcpServers are not supported by this agent',
      );
    }
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

/// Delays a stream's done event until every tracked task has completed.
///
/// json_rpc_2 completes its server future as soon as the input stream ends;
/// a response whose handler is still running when EOF arrives would be
/// discarded. The gate holds the stream open until all in-flight request
/// handlers finish, so their responses are written before the connection
/// closes.
final class _InFlightGate {
  final _controller = StreamController<String>();
  int _pending = 0;
  bool _inputDone = false;
  bool _doneScheduled = false;

  /// Wraps [input], mirroring its events but delaying `done` until no
  /// tracked task is pending.
  Stream<String> wrap(Stream<String> input) {
    input.listen(
      _controller.add,
      onError: _controller.addError,
      onDone: () {
        _inputDone = true;
        _maybeDone();
      },
    );
    return _controller.stream;
  }

  /// Runs [action] while keeping the connection open.
  Future<T> run<T>(Future<T> Function() action) {
    _pending++;
    return Future<T>.sync(action).whenComplete(() {
      _pending--;
      _maybeDone();
    });
  }

  void _maybeDone() {
    if (!_inputDone || _pending != 0 || _doneScheduled) {
      return;
    }
    _doneScheduled = true;
    // Defer past the current microtask so the finishing handler's response
    // is written before json_rpc_2 observes the closed input stream.
    scheduleMicrotask(() {
      if (!_controller.isClosed) {
        _controller.close();
      }
    });
  }
}

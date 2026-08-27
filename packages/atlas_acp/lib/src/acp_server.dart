import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:acpd/acpd.dart';
import 'package:acpd_io/acpd_io.dart' show StdioTransport;
import 'package:atlas_runtime/atlas_runtime.dart' as rt;
import 'package:stream_channel/stream_channel.dart';

import 'acp_types.dart';
import 'transport.dart';
import 'update_mapper.dart';

/// The ACP agent adapter for Atlas.
///
/// Exposes the runtime to ACP clients (editors such as Zed) over NDJSON
/// stdio. The adapter maps protocol methods to runtime calls and streams
/// runtime events back as `session/update` notifications. The JSON-RPC
/// engine is provided by acpd; Atlas owns the runtime mapping.
final class AcpServer {
  /// Creates an ACP server over [runtime].
  ///
  /// [models] is the configured model catalog offered through session
  /// `configOptions`; when empty, only the runtime default model is shown.
  AcpServer(this.runtime, {this.models = const <rt.ModelDescriptor>[]});

  /// The runtime serving ACP sessions.
  final rt.AgentEngine runtime;

  /// The configured model catalog, in display priority order.
  final List<rt.ModelDescriptor> models;

  /// Last title reported to the client per session, used to emit
  /// `session_info_update` when the runtime auto-generates a title.
  final _titles = <String, String>{};

  /// Current model and reasoning effort per session, set on session/new,
  /// load, and resume, and changed through `session/set_config_option`.
  final _sessionConfigs = <String, _SessionConfig>{};

  /// Cancellation tokens of pending turns per session. The runtime serializes
  /// turns per session, so several prompts may be queued behind one active
  /// turn; cancelling must reach all of them.
  final _activeTurns = <String, List<rt.CancellationToken>>{};

  /// Pending deferred lifecycle notifications, flushed before the connection
  /// closes so clients can receive them.
  final _deferred = <Future<void>>{};

  /// Message id counter for `/compact` result messages, which are not part of
  /// a model turn and need unique ids across invocations.
  int _compactCounter = 0;

  AgentContext? _context;
  bool _debug = false;
  String? _clientName;
  InFlightTransport? _gate;

  /// Serves ACP over [input] and [output], completing when the connection
  /// closes.
  ///
  /// Defaults to the process stdin/stdout. Logging must go to stderr; stdout
  /// carries only protocol messages.
  Future<void> serve({Stream<List<int>>? input, IOSink? output}) async {
    _debug = Platform.environment['ACP_DEBUG'] == '1';
    final stream = input ?? stdin;
    final sink = output ?? stdout;
    final transport = StdioTransport(
      input: stream,
      writeOutput: (bytes) => sink.add(bytes),
      onClose: () async {
        await sink.flush();
      },
      onSend: (line) {
        if (_debug) stderr.writeln('atlas_acp: >> $line');
      },
      onReceive: (line) {
        if (_debug) stderr.writeln('atlas_acp: << $line');
      },
    );
    await _serveTransport(transport);
  }

  /// Serves over an existing string channel, exposed for tests.
  Future<void> serveChannel(StreamChannel<String> channel) async {
    await _serveTransport(ChannelTransport(channel));
  }

  /// Serves ACP over an in-process transport, returning the connection
  /// completion future and the client-side transport.
  ///
  /// The returned future completes when the connection closes.
  (Future<void>, Transport) serveMemory() {
    final pair = MemoryTransportPair();
    return (_serveTransport(pair.right), pair.left);
  }

  Future<void> _serveTransport(Transport transport) async {
    // Defer EOF until in-flight handlers finish so their responses are
    // written before the connection closes.
    final gate = InFlightTransport(transport);
    final role = AgentRole()
      ..onInitialize(_requestLog('initialize', _initialize))
      ..onNewSession(_requestLog('session/new', _newSession))
      ..onLoadSession(_requestLog('session/load', _loadSession))
      ..onResumeSession(_requestLog('session/resume', _resumeSession))
      ..onPrompt(_requestLog('session/prompt', _prompt))
      ..onListSessions(_requestLog('session/list', _listSessions))
      ..onCloseSession(_requestLog('session/close', _closeSession))
      ..onDeleteSession(_requestLog('session/delete', _deleteSession))
      ..onSetSessionConfigOption(
        _requestLog('session/set_config_option', _setConfigOption),
      )
      ..onSessionCancel(_cancelPrompt)
      ..handle(acpSessionSetTitleMethod, (params, context) async {
        try {
          return await _setTitle(params);
        } on RpcError {
          rethrow;
        } catch (error) {
          throw _internalError(error);
        }
      })
      ..handle(acpSessionCompactMethod, (params, context) async {
        final sessionId = params['sessionId'];
        if (sessionId is! String || sessionId.isEmpty) {
          throw RpcError(code: -32602, message: 'sessionId is required');
        }
        final instruction = params['instruction'] as String?;
        var kept = 0;
        var before = 0;
        var after = 0;
        var summaryPresent = false;
        await for (final event in runtime.compact(
          rt.SessionId(sessionId),
          instruction: instruction,
        )) {
          switch (event) {
            case rt.CompactionFinished(:final checkpoint):
              kept = checkpoint.keptRecentMessages;
              before = checkpoint.inputTokensBefore;
              after = checkpoint.inputTokensAfter;
              summaryPresent = checkpoint.summary.isNotEmpty;
            default:
              break;
          }
        }
        return <String, Object?>{
          'keptMessages': kept,
          'tokensBefore': before,
          'tokensAfter': after,
          'summaryPresent': summaryPresent,
        };
      });
    final connection = role.connect(gate);
    _context = connection.agent;
    _gate = gate;
    await connection.closed;
    // Flush deferred lifecycle notifications so the connection closes only
    // after the client could have received them.
    await Future.wait(_deferred.toList());
  }

  /// Wraps [handler] with request/response logging when ACP_DEBUG is set and
  /// converts unexpected failures into internal RPC errors.
  FutureOr<R> Function(
    AgentContext context,
    P params,
    RequestCancellation cancellation,
  )
  _requestLog<P, R>(String method, FutureOr<R> Function(P params) handler) {
    return (context, params, cancellation) {
      final sessionId = switch (params) {
        LoadSessionRequest(:final sessionId) => sessionId,
        ResumeSessionRequest(:final sessionId) => sessionId,
        PromptRequest(:final sessionId) => sessionId,
        CloseSessionRequest(:final sessionId) => sessionId,
        DeleteSessionRequest(:final sessionId) => sessionId,
        SetSessionConfigOptionRequest(:final sessionId) => sessionId,
        _ => null,
      };
      if (_debug) {
        stderr.writeln(
          'atlas_acp: << $method'
          '${sessionId == null ? '' : ' session=$sessionId'}',
        );
      }
      try {
        final result = handler(params);
        if (result is Future<R>) {
          final tracked = _gate?.run(() => result) ?? result;
          return tracked
              .then((value) {
                if (_debug) stderr.writeln('atlas_acp: >> $method: ok');
                return value;
              })
              .catchError((Object error, StackTrace stackTrace) {
                if (_debug) {
                  stderr.writeln(
                    'atlas_acp: >> $method: '
                    '${error is RpcError ? 'error ${error.code} ${error.message}' : error.runtimeType}',
                  );
                }
                if (error is RpcError) {
                  throw error;
                }
                throw _internalError(error);
              });
        }
        if (_debug) stderr.writeln('atlas_acp: >> $method: ok');
        return result;
      } catch (error) {
        if (_debug) {
          stderr.writeln(
            'atlas_acp: >> $method: '
            '${error is RpcError ? 'error ${error.code} ${error.message}' : error.runtimeType}',
          );
        }
        if (error is RpcError) {
          rethrow;
        }
        throw _internalError(error);
      }
    };
  }

  /// Builds a safe internal-error RpcError for [error].
  static RpcError _internalError(Object error) {
    final detail = error is rt.SafeMessageException
        ? ': ${error.safeMessage}'
        : '';
    return RpcError(
      code: -32603,
      message: 'internal error (${error.runtimeType}$detail)',
    );
  }

  Future<InitializeResponse> _initialize(InitializeRequest params) async {
    _clientName = params.clientInfo?.name;
    if (_debug && _clientName != null) {
      stderr.writeln('atlas_acp: client=$_clientName');
    }
    return initializeResult();
  }

  Future<NewSessionResponse> _newSession(NewSessionRequest params) async {
    _rejectMcpServers(params.mcpServers);
    final cwd = _absolutePath(params.cwd, 'cwd');
    final additionalDirectories = [
      for (final entry in params.additionalDirectories)
        _absolutePath(entry, 'additionalDirectories'),
    ];
    final session = await runtime.createSession(
      workingDirectory: cwd,
      additionalDirectories: additionalDirectories,
    );
    _titles[session.id.value] = session.title;
    final config = _defaultConfig(cwd);
    _sessionConfigs[session.id.value] = config;
    await runtime.updateSessionConfig(session.id, config.model, config.effort);
    _sendAvailableCommandsLater(session.id, cwd);
    return NewSessionResponse(
      sessionId: session.id.value,
      configOptions: sessionConfigOptions(models, config.model, config.effort),
    );
  }

  Future<LoadSessionResponse> _loadSession(LoadSessionRequest params) async {
    _rejectMcpServers(params.mcpServers);
    _absolutePath(params.cwd, 'cwd');
    final session = rt.SessionId(params.sessionId);
    final snapshot = await _load(session);
    _titles[snapshot.session.id.value] = snapshot.session.title;
    final config = _configForSnapshot(snapshot);
    _sessionConfigs[snapshot.session.id.value] = config;
    for (final update in replayTimeline(
      snapshot.timeline,
      workingDirectory: snapshot.session.workingDirectory,
    )) {
      _sendUpdate(snapshot.session.id, update);
    }
    _sendAvailableCommandsLater(
      snapshot.session.id,
      snapshot.session.workingDirectory,
    );
    // Report the loaded session's context occupancy instead of leaving the
    // client without a usage figure until the next turn completes.
    final usage = _estimatedSessionUsage(snapshot);
    if (usage != null) {
      await _bestEffort(() => _sendUsageUpdate(snapshot.session.id, usage));
    }
    return LoadSessionResponse(
      configOptions: sessionConfigOptions(models, config.model, config.effort),
    );
  }

  Future<LoadSessionResponse> _resumeSession(
    ResumeSessionRequest params,
  ) async {
    _rejectMcpServers(params.mcpServers);
    _absolutePath(params.cwd, 'cwd');
    final session = rt.SessionId(params.sessionId);
    final snapshot = await _load(session);
    _titles[snapshot.session.id.value] = snapshot.session.title;
    final config = _configForSnapshot(snapshot);
    _sessionConfigs[snapshot.session.id.value] = config;
    _sendAvailableCommandsLater(
      snapshot.session.id,
      snapshot.session.workingDirectory,
    );
    // Same as session/load: surface the context occupancy immediately.
    final usage = _estimatedSessionUsage(snapshot);
    if (usage != null) {
      await _bestEffort(() => _sendUsageUpdate(snapshot.session.id, usage));
    }
    return LoadSessionResponse(
      configOptions: sessionConfigOptions(models, config.model, config.effort),
    );
  }

  Future<PromptResponse> _prompt(PromptRequest params) async {
    final sessionId = params.sessionId;
    if (sessionId.isEmpty) {
      throw RpcError(code: -32602, message: 'sessionId must not be empty');
    }
    final session = rt.SessionId(sessionId);
    final content = _promptContent(params.prompt);
    final promptText = _promptText(content);
    final compactInstruction = compactCommandInstruction(promptText);
    if (compactInstruction != null) {
      if (content.any((part) => part is rt.ImageContent)) {
        throw RpcError(
          code: -32602,
          message: 'slash commands do not support images',
        );
      }
      // Validate the session up front so an unknown session reports invalid
      // params like any other prompt.
      await _configFor(session);
      return _runCompact(session, compactInstruction);
    }
    final cancellation = rt.CancellationToken();
    (_activeTurns[sessionId] ??= <rt.CancellationToken>[]).add(cancellation);
    try {
      // The runtime serializes turns per session, so a prompt sent while
      // another turn is running waits for it instead of failing.
      final config = _sessionConfigs[sessionId] ?? await _configFor(session);
      final mapper = TurnUpdateMapper(session, workingDirectory: config.cwd);
      final skills = _matchedSkillNames(config.cwd, promptText);
      return await _runPrompt(
        session,
        content,
        config,
        mapper,
        skills,
        cancellation,
      );
    } on rt.SessionNotFoundException catch (error) {
      throw RpcError(
        code: -32602,
        message: 'session not found: ${error.sessionId}',
      );
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

  /// Runs one prompt turn to completion, streaming updates as they occur.
  Future<PromptResponse> _runPrompt(
    rt.SessionId session,
    List<rt.ContentPart> content,
    _SessionConfig config,
    TurnUpdateMapper mapper,
    List<String> skills,
    rt.CancellationToken cancellation,
  ) async {
    String? stopReason;
    String? failureCode;
    String? failureMessage;
    int? usedTokens;
    await for (final event in runtime.run(
      rt.TurnRequest(
        sessionId: session,
        content: content,
        model: config.model,
        reasoningEffort: config.effort,
        skills: skills,
        cancellation: cancellation,
      ),
    )) {
      for (final update in mapper.map(event)) {
        _sendUpdate(session, update);
      }
      if (event case rt.TurnFinished(:final outcome)) {
        switch (outcome.status) {
          case rt.TurnStatus.completed:
            stopReason = outcome.stopReason == rt.StopReason.maxTokens
                ? 'max_tokens'
                : 'end_turn';
            usedTokens = outcome.usage.inputTokens > 0
                ? outcome.usage.inputTokens
                : outcome.usage.totalTokens;
          case rt.TurnStatus.cancelled:
            stopReason = 'cancelled';
          case rt.TurnStatus.failed:
            failureCode = outcome.failure?.code;
            failureMessage = outcome.failure?.message;
          case rt.TurnStatus.running:
            break;
        }
      }
      if (event case rt.CompactionFinished(:final checkpoint)) {
        // The trailing compaction shrinks the context; report the
        // post-compaction usage instead of the turn's pre-compaction value.
        usedTokens = checkpoint.inputTokensAfter;
      }
    }
    // Consume the whole stream (including trailing compaction events)
    // before reporting the failure so the runtime turn settles.
    if (failureCode != null) {
      throw RpcError(
        code: -32603,
        message: failureMessage == null
            ? 'turn failed ($failureCode)'
            : 'turn failed ($failureCode): $failureMessage',
      );
    }
    if (stopReason == null) {
      throw RpcError(code: -32603, message: 'turn ended without a stop reason');
    }
    // Title and usage notifications are best-effort: a failure here must
    // not fail a turn that already completed.
    await _bestEffort(() => _notifyTitleChange(session));
    await _bestEffort(() => _sendUsageUpdate(session, usedTokens));
    return PromptResponse(stopReason: _stopReason(stopReason));
  }

  StopReason _stopReason(String value) => switch (value) {
    'max_tokens' => StopReason.maxTokens,
    'cancelled' => StopReason.cancelled,
    _ => StopReason.endTurn,
  };

  void _cancelPrompt(AgentContext context, CancelNotification params) {
    final sessionId = params.sessionId;
    for (final token
        in _activeTurns[sessionId] ?? const <rt.CancellationToken>[]) {
      token.cancel();
    }
  }

  Future<ListSessionsResponse> _listSessions(ListSessionsRequest params) async {
    final cwd = params.cwd;
    if (cwd != null) {
      _absolutePath(cwd, 'cwd');
    }
    final cursor = params.cursor;
    final page = await runtime.listSessions(
      workingDirectory: cwd,
      cursor: cursor,
    );
    // Record listed titles so a later prompt does not re-send a title the
    // client already saw in the list response.
    for (final item in page.items) {
      _titles[item.id.value] = item.title;
    }
    return ListSessionsResponse(
      sessions: [
        for (final session in page.items)
          SessionInfo(
            sessionId: session.id.value,
            cwd: session.workingDirectory,
            additionalDirectories: session.additionalDirectories,
            title: session.title.isEmpty ? null : session.title,
            updatedAt: session.updatedAt.toUtc().toIso8601String(),
          ),
      ],
      nextCursor: page.nextCursor,
    );
  }

  Future<CloseSessionResponse> _closeSession(CloseSessionRequest params) async {
    final sessionId = params.sessionId;
    for (final token
        in _activeTurns[sessionId] ?? const <rt.CancellationToken>[]) {
      token.cancel();
    }
    return CloseSessionResponse();
  }

  Future<DeleteSessionResponse> _deleteSession(
    DeleteSessionRequest params,
  ) async {
    final sessionId = params.sessionId;
    for (final token
        in _activeTurns[sessionId] ?? const <rt.CancellationToken>[]) {
      token.cancel();
    }
    try {
      await runtime.deleteSession(rt.SessionId(sessionId));
    } on rt.SessionNotFoundException {
      // Deleting an unknown or already-deleted session succeeds silently.
    }
    _titles.remove(sessionId);
    _sessionConfigs.remove(sessionId);
    return DeleteSessionResponse();
  }

  /// Renames [params.sessionId] through the runtime and notifies the client.
  Future<Map<String, Object?>> _setTitle(Map<String, Object?> params) async {
    final sessionId = params['sessionId'];
    final title = params['title'];
    if (sessionId is! String || sessionId.isEmpty) {
      throw RpcError(code: -32602, message: 'sessionId must not be empty');
    }
    if (title is! String || title.trim().isEmpty) {
      throw RpcError(code: -32602, message: 'title must not be empty');
    }
    final trimmed = title.trim();
    try {
      await runtime.renameSession(rt.SessionId(sessionId), trimmed);
    } on rt.SessionNotFoundException catch (error) {
      throw RpcError(
        code: -32602,
        message: 'session not found: ${error.sessionId}',
      );
    }
    _titles[sessionId] = trimmed;
    _sendUpdate(
      rt.SessionId(sessionId),
      SessionInfoSessionUpdate(title: trimmed),
    );
    return const <String, Object?>{};
  }

  Future<SetSessionConfigOptionResponse> _setConfigOption(
    SetSessionConfigOptionRequest params,
  ) async {
    final sessionId = params.sessionId;
    final configId = params.configId;
    final config = _sessionConfigs[sessionId];
    if (config == null) {
      throw RpcError(code: -32602, message: 'session not found: $sessionId');
    }
    final value = switch (params) {
      SetValueIdConfigOption(:final value) => value,
      _ => throw RpcError(
        code: -32602,
        message: 'unsupported session config option: $configId',
      ),
    };
    final updated = switch (configId) {
      acpConfigIdModel => _setModel(config, value),
      acpConfigIdEffort => _setReasoningEffort(config, value),
      _ => throw RpcError(
        code: -32602,
        message: 'unsupported session config option: $configId',
      ),
    };
    _sessionConfigs[sessionId] = updated;
    await runtime.updateSessionConfig(
      rt.SessionId(sessionId),
      updated.model,
      updated.effort,
    );
    return SetSessionConfigOptionResponse(
      configOptions: sessionConfigOptions(
        models,
        updated.model,
        updated.effort,
      ),
    );
  }

  /// Returns the session config for [config] with the model switched to
  /// [value], resetting the reasoning effort when the new model does not
  /// support the current one.
  _SessionConfig _setModel(_SessionConfig config, String value) {
    final descriptor = _descriptorByValue(value);
    if (descriptor == null) {
      throw RpcError(code: -32602, message: 'unknown model: $value');
    }
    final effort = config.effort;
    final supportsEffort =
        effort != null &&
        descriptor.reasoningEfforts.any((option) => option.value == effort);
    return _SessionConfig(
      cwd: config.cwd,
      model: descriptor.ref,
      effort: supportsEffort
          ? effort
          : (descriptor.reasoningEfforts.isEmpty
                ? null
                : descriptor.reasoningEfforts.first.value),
    );
  }

  /// Returns the session config for [config] with the reasoning effort
  /// switched to [value], which must be supported by the current model.
  _SessionConfig _setReasoningEffort(_SessionConfig config, String value) {
    final descriptor = _descriptorFor(config.model);
    if (!descriptor.reasoningEfforts.any((option) => option.value == value)) {
      throw RpcError(
        code: -32602,
        message: 'unsupported reasoning effort for ${config.model}: $value',
      );
    }
    return _SessionConfig(cwd: config.cwd, model: config.model, effort: value);
  }

  /// The default session config for [cwd]: the runtime default model with
  /// its first reasoning effort when the model declares any.
  _SessionConfig _defaultConfig(String cwd) {
    final descriptor = _descriptorFor(runtime.defaultModel);
    return _SessionConfig(
      cwd: cwd,
      model: runtime.defaultModel,
      effort: descriptor.reasoningEfforts.isEmpty
          ? null
          : descriptor.reasoningEfforts.first.value,
    );
  }

  /// Restores the last model and effort selected for a persisted session.
  _SessionConfig _configForSnapshot(rt.SessionSnapshot snapshot) {
    final fallback = _defaultConfig(snapshot.session.workingDirectory);
    if (snapshot.session.model != null) {
      return _SessionConfig(
        cwd: snapshot.session.workingDirectory,
        model: snapshot.session.model!,
        effort: snapshot.session.reasoningEffort,
      );
    }
    for (final turn in snapshot.turns.reversed) {
      final model = turn.model;
      if (model != null) {
        return _SessionConfig(
          cwd: snapshot.session.workingDirectory,
          model: model,
          effort: turn.reasoningEffort,
        );
      }
    }
    return fallback;
  }

  /// The catalog descriptor for [ref], or a bare descriptor when the catalog
  /// does not include it.
  rt.ModelDescriptor _descriptorFor(rt.ModelRef ref) {
    for (final model in models) {
      if (model.ref == ref) {
        return model;
      }
    }
    return rt.ModelDescriptor(ref: ref);
  }

  /// The catalog descriptor whose `<provider>/<model>` value is [value], or
  /// null when no configured model matches.
  rt.ModelDescriptor? _descriptorByValue(String value) {
    for (final model in models) {
      if (model.ref.toString() == value) {
        return model;
      }
    }
    return null;
  }

  /// Emits `session_info_update` when the runtime has auto-generated a title
  /// (from the first user message) that the client has not seen yet.
  Future<void> _notifyTitleChange(rt.SessionId session) async {
    try {
      final title = runtime is rt.AgentRuntime
          ? (await runtime.loadSessionMetadata(session)).title
          : (await runtime.loadSession(session)).session.title;
      if (title.isNotEmpty && title != _titles[session.value]) {
        _titles[session.value] = title;
        _sendUpdate(session, SessionInfoSessionUpdate(title: title));
      }
    } on rt.SessionNotFoundException {
      // The session was deleted mid-turn; there is nothing to update.
    }
  }

  /// Runs the `/compact` slash command: manually compacts [session] and
  /// reports the outcome as an assistant message, without a model turn.
  ///
  /// [instruction] is the optional user text after `/compact`, forwarded to
  /// the runtime compaction summary request.
  Future<PromptResponse> _runCompact(
    rt.SessionId session,
    String instruction,
  ) async {
    final sessionId = session.value;
    final cancellation = rt.CancellationToken();
    (_activeTurns[sessionId] ??= <rt.CancellationToken>[]).add(cancellation);
    try {
      var keptMessages = -1;
      int? compactedUsage;
      var failed = false;
      await for (final event in runtime.compact(
        session,
        instruction: instruction,
        cancellation: cancellation,
      )) {
        cancellation.throwIfCancelled();
        switch (event) {
          case rt.CompactionFinished(:final checkpoint):
            keptMessages = checkpoint.keptRecentMessages;
            compactedUsage = checkpoint.inputTokensAfter;
          case rt.CompactionFailed():
            failed = true;
          default:
            break;
        }
      }
      final message = switch ((keptMessages, failed)) {
        (final kept, _) when kept >= 0 =>
          'Context compacted. Kept $kept recent messages.',
        (_, true) => 'Compaction failed.',
        _ => 'No safe context to compact.',
      };
      _sendUpdate(
        session,
        AgentMessageChunk(
          chunk: ContentChunk(
            messageId: 'msg-compact-${_compactCounter++}',
            content: TextContentBlock(text: message),
            meta: {
              'atlas.dev': {
                'compact': {
                  'status': failed
                      ? 'failed'
                      : keptMessages >= 0
                      ? 'completed'
                      : 'no_op',
                  'keptMessages': ?(keptMessages >= 0 ? keptMessages : null),
                  'tokensAfter': ?compactedUsage,
                },
              },
            },
          ),
        ),
      );
      // Report the post-compaction context usage so clients show the freed
      // space instead of the pre-compaction occupancy.
      if (compactedUsage != null) {
        await _bestEffort(() => _sendUsageUpdate(session, compactedUsage));
      }
      return PromptResponse(stopReason: StopReason.endTurn);
    } on rt.TurnCancelledException {
      return PromptResponse(stopReason: StopReason.cancelled);
    } on rt.SessionNotFoundException catch (error) {
      throw RpcError(
        code: -32602,
        message: 'session not found: ${error.sessionId}',
      );
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

  /// The session config for [session], loading the stored working directory
  /// when the session was not configured in this process.
  Future<_SessionConfig> _configFor(rt.SessionId session) async {
    final snapshot = await _load(session);
    return _configForSnapshot(snapshot);
  }

  Future<rt.SessionSnapshot> _load(rt.SessionId session) async {
    try {
      return await runtime.loadSession(session);
    } on rt.SessionNotFoundException catch (error) {
      throw RpcError(
        code: -32602,
        message: 'session not found: ${error.sessionId}',
      );
    }
  }

  /// The plain text of [content], used for slash command recognition.
  static String _promptText(List<rt.ContentPart> content) =>
      content.whereType<rt.TextContent>().map((part) => part.text).join('\n');

  /// The skill names referenced by `/name` tokens in [promptText], limited
  /// to skills available in [cwd] and in order of first appearance.
  List<String> _matchedSkillNames(String cwd, String promptText) {
    final known = {for (final skill in _skillsFor(cwd)) skill.name};
    if (known.isEmpty) {
      return const [];
    }
    return matchedCommandNames(promptText, known);
  }

  /// The skill summaries available in [cwd], or an empty list when the
  /// session context cannot be built.
  List<rt.SkillSummary> _skillsFor(String cwd) {
    try {
      return runtime.sessionContext(cwd).skills.summaries;
    } catch (_) {
      return const [];
    }
  }

  /// Sends the `available_commands_update` notification after [session] is
  /// registered, matching the ACP client lifecycle.
  void _sendAvailableCommandsLater(rt.SessionId session, String cwd) {
    // Defer past the response: clients may not have registered the session
    // when the notification would otherwise arrive first.
    final future = Future<void>(() {
      try {
        final known = _skillsFor(cwd);
        _sendUpdate(
          session,
          AvailableCommandsSessionUpdate(
            update: AvailableCommandsUpdate(
              availableCommands: availableCommandsFor(known),
            ),
          ),
        );
      } catch (_) {
        // The connection may already be closing; the notification is
        // optional and must never surface as an unhandled error.
      }
    });
    _deferred.add(future);
    future.whenComplete(() => _deferred.remove(future));
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
  Future<void> _sendUsageUpdate(rt.SessionId session, int? usedTokens) async {
    if (usedTokens == null || usedTokens <= 0) {
      return;
    }
    final size = await _contextWindow();
    if (size <= 0) {
      return;
    }
    _sendUpdate(session, UsageSessionUpdate(used: usedTokens, size: size));
  }

  /// The best known context occupancy for a loaded session, or null when the
  /// session carries no usage information.
  ///
  /// With a compaction checkpoint, the post-compaction estimate is the
  /// baseline plus an estimate of the timeline added after the checkpoint;
  /// otherwise the whole timeline is estimated.
  int? _estimatedSessionUsage(rt.SessionSnapshot snapshot) {
    final checkpoint = snapshot.session.compaction;
    if (checkpoint != null && checkpoint.inputTokensAfter > 0) {
      final active = snapshot.timeline
          .where((item) => item.sequence > checkpoint.compactedThroughSequence)
          .toList();
      return checkpoint.inputTokensAfter + _estimateTimelineTokens(active);
    }
    final estimate = _estimateTimelineTokens(snapshot.timeline);
    return estimate > 0 ? estimate : null;
  }

  /// A rough token estimate for [items] using the same four-characters-per-
  /// token ratio as the runtime compaction accounting.
  static int _estimateTimelineTokens(List<rt.TimelineItem> items) {
    final buffer = StringBuffer();
    for (final item in items) {
      switch (item) {
        case rt.UserMessageItem(:final content):
        case rt.AssistantMessageItem(:final content):
          buffer.write(rt.textFromContent(content));
        case rt.ToolCallItem(:final call):
          buffer
            ..write(call.name)
            ..write(jsonEncode(call.arguments));
        case rt.ToolResultItem(:final content):
          buffer.write(content);
      }
    }
    return rt.estimateTokenCount(buffer.toString());
  }

  /// Converts an ACP prompt array into runtime content parts.
  ///
  /// Atlas advertises text + image prompt capabilities, so those blocks are
  /// accepted; `resource` blocks are accepted once embeddedContext support
  /// advertises them. `resource_link` blocks are a baseline content type and
  /// are ignored: Atlas tools access the local filesystem directly and do not
  /// need client-provided resource contents.
  static List<rt.ContentPart> _promptContent(List<ContentBlock> blocks) {
    if (blocks.isEmpty) {
      throw RpcError(code: -32602, message: 'prompt must not be empty');
    }
    final parts = <rt.ContentPart>[];
    for (final block in blocks) {
      switch (block) {
        case TextContentBlock(:final text):
          if (text.isNotEmpty) {
            parts.add(rt.TextContent(text));
          }
        case ImageContent(:final data, :final mimeType):
          if (data.isEmpty) {
            throw RpcError(
              code: -32602,
              message: 'image block must contain data',
            );
          }
          parts.add(
            rt.ImageContent(
              source: 'data:$mimeType;base64,$data',
              mimeType: mimeType,
            ),
          );
        case ResourceLink(:final name, :final uri):
          parts.add(rt.TextContent('<resource name="$name" uri="$uri"/>'));
        case EmbeddedResource(:final resource):
          switch (resource) {
            case TextResourceContents(:final uri, :final text):
              parts.add(
                rt.ResourceContent(
                  uri: uri,
                  mimeType: resource.mimeType,
                  text: text,
                ),
              );
            default:
              throw RpcError(
                code: -32602,
                message: 'only text resource blocks are supported',
              );
          }
        default:
          throw RpcError(
            code: -32602,
            message: 'unsupported content block type: ${block.type}',
          );
      }
    }
    if (parts.isEmpty) {
      throw RpcError(
        code: -32602,
        message: 'prompt must contain text or image content',
      );
    }
    return parts;
  }

  /// Rejects a non-empty `mcpServers` parameter: Atlas does not implement
  /// MCP server connections (a MUST-level ACP stdio capability), so a client
  /// asking for them gets an explicit error instead of silent failure.
  static void _rejectMcpServers(List<McpServer> servers) {
    if (servers.isNotEmpty) {
      throw RpcError(
        code: -32602,
        message: 'mcpServers are not supported by this agent',
      );
    }
  }

  /// Validates that [path] is absolute, as required by ACP, and returns it.
  static String _absolutePath(String path, String field) {
    // Accepts POSIX and Windows drive-letter roots.
    final isAbsolute =
        path.startsWith('/') || RegExp(r'^[A-Za-z]:[\\/]').hasMatch(path);
    if (!isAbsolute) {
      throw RpcError(code: -32602, message: '$field must be an absolute path');
    }
    return path;
  }

  void _sendUpdate(rt.SessionId session, SessionUpdate update) {
    if (_debug) {
      stderr.writeln(
        'atlas_acp: ~ session=${session.value} update=${update.kind.value}',
      );
    }
    _context?.sessionUpdate(sessionId: session.value, update: update);
  }
}

/// The per-session model, reasoning effort, and working directory selection.
final class _SessionConfig {
  /// Creates a session config.
  const _SessionConfig({
    required this.cwd,
    required this.model,
    required this.effort,
  });

  /// The session working directory, used to resolve local skills.
  final String cwd;

  /// The selected model.
  final rt.ModelRef model;

  /// The selected reasoning effort, or null when the model has none.
  final String? effort;
}

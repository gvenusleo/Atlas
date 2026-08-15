import 'dart:async';
import 'dart:convert';
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
  ///
  /// [models] is the configured model catalog offered through session
  /// `configOptions`; when empty, only the runtime default model is shown.
  AcpServer(this.runtime, {this.models = const <ModelDescriptor>[]});

  /// The runtime serving ACP sessions.
  final AgentRuntime runtime;

  /// The configured model catalog, in display priority order.
  final List<ModelDescriptor> models;

  /// Last title reported to the client per session, used to emit
  /// `session_info_update` when the runtime auto-generates a title.
  final _titles = <String, String>{};

  /// Current model and reasoning effort per session, set on session/new,
  /// load, and resume, and changed through `session/set_config_option`.
  final _sessionConfigs = <String, _SessionConfig>{};

  /// Cancellation tokens of pending turns per session. The runtime serializes
  /// turns per session, so several prompts may be queued behind one active
  /// turn; cancelling must reach all of them.
  final _activeTurns = <String, List<CancellationToken>>{};

  /// Delays connection close until in-flight requests have responded, so EOF
  /// never drops a pending response.
  final _inFlight = _InFlightGate();

  /// Pending deferred lifecycle notifications, flushed before the connection
  /// closes so clients can receive them.
  final _deferred = <Future<void>>{};

  /// Message id counter for `/compact` result messages, which are not part of
  /// a model turn and need unique ids across invocations.
  int _compactCounter = 0;

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
    final peer = Peer(
      StreamChannel<String>(_inFlight.wrap(channel.stream), channel.sink),
      onUnhandledError: (error, stackTrace) {
        stderr.writeln('atlas_acp: unhandled error: $error');
      },
    );
    _registerMethods(peer);
    await peer.listen();
    // Flush deferred lifecycle notifications so the connection closes only
    // after the client could have received them.
    await Future.wait(_deferred.toList());
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
      ..registerMethod(
        'session/set_config_option',
        _requestLog(_setConfigOption),
      )
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

  Future<JsonObject> _initialize(Parameters params) async {
    return initializeResult();
  }

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
    final config = _defaultConfig(cwd);
    _sessionConfigs[session.id.value] = config;
    _sendAvailableCommandsLater(session.id, cwd);
    return {
      'sessionId': session.id.value,
      'configOptions': sessionConfigOptions(
        models,
        config.model,
        config.effort,
      ),
    };
  }

  Future<Object?> _loadSession(Parameters params) async {
    final snapshot = await _load(params);
    _titles[snapshot.session.id.value] = snapshot.session.title;
    final config = _defaultConfig(snapshot.session.workingDirectory);
    _sessionConfigs[snapshot.session.id.value] = config;
    for (final update in replayTimeline(
      snapshot.timeline,
      workingDirectory: snapshot.session.workingDirectory,
    )) {
      _sendUpdate(update);
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
    return {
      'configOptions': sessionConfigOptions(
        models,
        config.model,
        config.effort,
      ),
    };
  }

  Future<JsonObject> _resumeSession(Parameters params) async {
    final snapshot = await _load(params);
    _titles[snapshot.session.id.value] = snapshot.session.title;
    final config = _defaultConfig(snapshot.session.workingDirectory);
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
    return {
      'configOptions': sessionConfigOptions(
        models,
        config.model,
        config.effort,
      ),
    };
  }

  /// The best known context occupancy for a loaded session, or null when the
  /// session carries no usage information.
  ///
  /// With a compaction checkpoint, the post-compaction estimate is the
  /// baseline plus an estimate of the timeline added after the checkpoint;
  /// otherwise the whole timeline is estimated. The runtime does not persist
  /// per-turn usage today (stored `lastUsage` stays zero), so this must fall
  /// back to estimating the timeline; re-check this branch if the runtime
  /// starts persisting usage.
  int? _estimatedSessionUsage(SessionSnapshot snapshot) {
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
  ///
  /// The text is rendered without the XML markup the runtime uses for
  /// compaction transcripts, so this estimate runs slightly lower than the
  /// runtime's own accounting; both are approximations for display purposes.
  static int _estimateTimelineTokens(List<TimelineItem> items) {
    final buffer = StringBuffer();
    for (final item in items) {
      switch (item) {
        case UserMessageItem(:final content):
        case AssistantMessageItem(:final content):
          buffer.write(textFromContent(content));
        case ToolCallItem(:final call):
          buffer
            ..write(call.name)
            ..write(jsonEncode(call.arguments));
        case ToolResultItem(:final content):
          buffer.write(content);
      }
    }
    return buffer.length ~/ 4;
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
    final promptText = _promptText(content);
    final compactInstruction = compactCommandInstruction(promptText);
    if (compactInstruction != null) {
      if (content.any((part) => part is ImageContent)) {
        throw RpcException.invalidParams(
          'slash commands do not support images',
        );
      }
      // Validate the session up front so an unknown session reports invalid
      // params like any other prompt.
      await _configFor(session);
      return _runCompact(session, sessionId, compactInstruction);
    }
    final cancellation = CancellationToken();
    (_activeTurns[sessionId] ??= <CancellationToken>[]).add(cancellation);
    // The runtime serializes turns per session, so a prompt sent while
    // another turn is running waits for it instead of failing.
    final config = _sessionConfigs[sessionId] ?? await _configFor(session);
    final mapper = TurnUpdateMapper(session, workingDirectory: config.cwd);
    final skills = _matchedSkillNames(config.cwd, promptText);
    try {
      String? stopReason;
      String? failureCode;
      String? failureMessage;
      int? usedTokens;
      await for (final event in runtime.run(
        TurnRequest(
          sessionId: session,
          content: content,
          model: config.model,
          reasoningEffort: config.effort,
          skills: skills,
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
              failureMessage = outcome.failure?.message;
            case TurnStatus.running:
              break;
          }
        }
        if (event case CompactionFinished(:final checkpoint)) {
          // The trailing compaction shrinks the context; report the
          // post-compaction usage instead of the turn's pre-compaction value.
          usedTokens = checkpoint.inputTokensAfter;
        }
      }
      // Consume the whole stream (including trailing compaction events)
      // before reporting the failure so the runtime turn settles.
      if (failureCode != null) {
        throw RpcException(
          -32603,
          failureMessage == null
              ? 'turn failed ($failureCode)'
              : 'turn failed ($failureCode): $failureMessage',
        );
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
      // Never let raw exceptions reach json_rpc_2's default serializer,
      // which embeds the full stack trace in the error data. Failures with
      // a safe message surface it; everything else reports the type only.
      final detail = error is SafeMessageException
          ? ': ${error.safeMessage}'
          : '';
      throw RpcException(-32603, 'turn failed (${error.runtimeType}$detail)');
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
    _sessionConfigs.remove(sessionId);
    return <String, Object?>{};
  }

  /// Handles `session/set_config_option`, updating the session's model or
  /// reasoning effort and returning the complete new configuration state.
  Future<JsonObject> _setConfigOption(Parameters params) async {
    final sessionId = params['sessionId'].asString;
    final configId = params['configId'].asString;
    final rawValue = params['value'].valueOr(null);
    final config = _sessionConfigs[sessionId];
    if (config == null) {
      throw RpcException.invalidParams('session not found: $sessionId');
    }
    if (rawValue is! String || rawValue.isEmpty) {
      throw RpcException.invalidParams('value must be a non-empty string');
    }
    final updated = switch (configId) {
      acpConfigIdModel => _setModel(config, rawValue),
      acpConfigIdReasoningEffort => _setReasoningEffort(config, rawValue),
      _ => throw RpcException.invalidParams(
        'unsupported session config option: $configId',
      ),
    };
    _sessionConfigs[sessionId] = updated;
    return {
      'configOptions': sessionConfigOptions(
        models,
        updated.model,
        updated.effort,
      ),
    };
  }

  /// Returns the session config for [config] with the model switched to
  /// [value], resetting the reasoning effort when the new model does not
  /// support the current one.
  _SessionConfig _setModel(_SessionConfig config, String value) {
    final descriptor = _descriptorByValue(value);
    if (descriptor == null) {
      throw RpcException.invalidParams('unknown model: $value');
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
      throw RpcException.invalidParams(
        'unsupported reasoning effort for ${config.model}: $value',
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

  /// The catalog descriptor for [ref], or a bare descriptor when the catalog
  /// does not include it.
  ModelDescriptor _descriptorFor(ModelRef ref) {
    for (final model in models) {
      if (model.ref == ref) {
        return model;
      }
    }
    return ModelDescriptor(ref: ref);
  }

  /// The catalog descriptor whose `<provider>/<model>` value is [value], or
  /// null when no configured model matches.
  ModelDescriptor? _descriptorByValue(String value) {
    for (final model in models) {
      if (model.ref.toString() == value) {
        return model;
      }
    }
    return null;
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

  /// Runs the `/compact` slash command: manually compacts [session] and
  /// reports the outcome as an assistant message, without a model turn.
  ///
  /// [instruction] is the optional user text after `/compact`, forwarded to
  /// the runtime compaction summary request.
  Future<JsonObject> _runCompact(
    SessionId session,
    String sessionId,
    String instruction,
  ) async {
    final cancellation = CancellationToken();
    (_activeTurns[sessionId] ??= <CancellationToken>[]).add(cancellation);
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
          case CompactionFinished(:final checkpoint):
            keptMessages = checkpoint.keptRecentMessages;
            compactedUsage = checkpoint.inputTokensAfter;
          case CompactionFailed():
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
        agentMessageChunk(
          session,
          messageId: 'msg-compact-${_compactCounter++}',
          text: message,
        ),
      );
      // Report the post-compaction context usage so clients show the freed
      // space instead of the pre-compaction occupancy.
      if (compactedUsage != null) {
        await _bestEffort(() => _sendUsageUpdate(session, compactedUsage));
      }
      return {'stopReason': 'end_turn'};
    } on TurnCancelledException {
      return {'stopReason': 'cancelled'};
    } on SessionNotFoundException catch (error) {
      // The session was deleted while the manual compaction ran.
      throw RpcException.invalidParams('session not found: ${error.sessionId}');
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
  Future<_SessionConfig> _configFor(SessionId session) async {
    final SessionSnapshot snapshot;
    try {
      snapshot = await runtime.loadSession(session);
    } on SessionNotFoundException catch (error) {
      throw RpcException.invalidParams('session not found: ${error.sessionId}');
    }
    return _defaultConfig(snapshot.session.workingDirectory);
  }

  /// The plain text of [content], used for slash command recognition.
  static String _promptText(List<ContentPart> content) =>
      content.whereType<TextContent>().map((part) => part.text).join('\n');

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
  List<SkillSummary> _skillsFor(String cwd) {
    try {
      return runtime.sessionContextBuilder(cwd).skills.summaries;
    } catch (_) {
      return const [];
    }
  }

  /// Sends the `available_commands_update` notification after [session] is
  /// registered, matching the ACP client lifecycle.
  void _sendAvailableCommandsLater(SessionId session, String cwd) {
    // Defer past the response: clients may not have registered the session
    // when the notification would otherwise arrive first.
    final future = Future<void>(() {
      try {
        final known = _skillsFor(cwd);
        _sendUpdate(
          availableCommandsUpdate(session, availableCommandsFor(known)),
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
  final ModelRef model;

  /// The selected reasoning effort, or null when the model has none.
  final String? effort;
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

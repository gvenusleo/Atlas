import 'dart:async';
import 'dart:io';

import 'package:acpd/acpd.dart';
import 'package:atlas_runtime/atlas_runtime.dart' as rt;
import 'package:stream_channel/stream_channel.dart';

import 'acp_types.dart';
import 'client_update_mapper.dart';
import 'transport.dart';

/// An ACP client that exposes any ACP server as a [AgentSession].
///
/// Connects over a [Transport] (stdio for local processes, in-memory for the
/// Flutter app), speaks the ACP JSON-RPC methods through acpd, and
/// reconstructs runtime events from `session/update` notifications.
/// Presentation code consumes it exactly like a local runtime.
///
/// Inbound agent requests (such as `session/request_permission`) are surfaced
/// through [PermissionPort] so presentation code can ask the user before
/// replying.
final class AcpClient
    implements
        rt.PresentationAgentSession,
        rt.PermissionPort,
        rt.AgentCapabilityProvider {
  /// Creates an ACP client over [transport].
  ///
  /// [catalog] and [defaultModel] seed the model list before the first
  /// `session/new`. Local Flutter bootstrap uses this so the composer shows
  /// the configured default instead of the synthetic `acp/default` placeholder.
  AcpClient(
    Transport transport, {
    List<rt.ModelDescriptor> catalog = const [],
    rt.ModelRef? defaultModel,
  }) : this._(transport, catalog: catalog, defaultModel: defaultModel);

  /// Creates an ACP client over a string channel, used by tests.
  AcpClient.channel(
    StreamChannel<String> channel, {
    List<rt.ModelDescriptor> catalog = const [],
    rt.ModelRef? defaultModel,
  }) : this._(
         ChannelTransport(channel),
         catalog: catalog,
         defaultModel: defaultModel,
       );

  AcpClient._(
    this._transport, {
    List<rt.ModelDescriptor> catalog = const [],
    rt.ModelRef? defaultModel,
  }) : _catalog = List.unmodifiable(catalog),
       _defaultModelRef = defaultModel ?? _syntheticDefaultModel.ref {
    for (final model in catalog) {
      if (model.reasoningEfforts.isNotEmpty) {
        _effortsByModel[model.ref] = model.reasoningEfforts;
      }
    }
  }

  /// The transport channel.
  final Transport _transport;

  final _updates = StreamController<SessionUpdateNotification>.broadcast(
    sync: true,
  );
  final _permissionRequests =
      StreamController<rt.PermissionRequest>.broadcast();
  final _pendingPermissions = <Object, Completer<rt.PermissionReply>>{};
  final _cancelledPermissions = <Object>{};
  int _permissionCounter = 0;
  ClientConnection? _connection;
  HandlerRegistration? _notificationReg;
  bool _connected = false;

  final _sessionCwd = <String, String>{};
  final _sessionModels = <String, rt.ModelRef>{};
  final _sessionEfforts = <String, String?>{};
  final _sessionUsage = <String, int>{};

  @override
  rt.AgentCapabilities get capabilities => const rt.AgentCapabilities(
    modes: true,
    slashCommands: true,
    rename: true,
    compact: true,
    permissions: true,
    images: true,
  );
  final _sessionTitles = <String, String>{};
  final _sessionCommands = <String, List<rt.AgentCommand>>{};
  final _sessionModes = <String, String>{};
  final _effortsByModel = <rt.ModelRef, List<rt.ReasoningEffortOption>>{};
  List<rt.ModelDescriptor> _catalog;
  List<rt.ModeOption> _modeOptions = const [];
  String _effortConfigId = acpConfigIdEffort;
  List<rt.AuthMethod> _authMethods = const [];
  rt.ModelRef _defaultModelRef;
  int _contextSize = 0;
  int _turnCounter = 0;

  /// The synthetic model used when no catalog is configured.
  static final _syntheticDefaultModel = rt.ModelDescriptor(
    ref: rt.ModelRef(
      providerId: rt.ProviderId('acp'),
      modelId: rt.ModelId('default'),
    ),
  );

  /// Connects to the ACP server and performs the `initialize` handshake.
  Future<void> connect() async {
    if (_connected) {
      return;
    }
    final role = ClientRole()..onRequestPermission(_handlePermissionRequest);
    _connection = role.connect(_transport);
    _notificationReg = _connection!.connection.onAnyNotification((
      method,
      params,
    ) {
      if (method == 'session/update' && params is Map<String, Object?>) {
        _onUpdate(SessionUpdateNotification.fromJson(params));
      }
    });
    // ACP requires the full initialize parameters; strict third-party
    // servers reject an empty payload with invalid-params.
    final response = await _client.initialize(
      const InitializeRequest(
        protocolVersion: ProtocolVersion.v1,
        clientInfo: Implementation(
          name: acpClientName,
          version: acpAgentVersion,
        ),
      ),
    );
    _authMethods = _parseAuthMethods(response.authMethods);
    _connected = true;
  }

  ClientContext get _client => _connection!.client;

  /// Closes the underlying connection and releases notification streams.
  Future<void> close() async {
    _notificationReg?.dispose();
    _notificationReg = null;
    // Reject any unanswered permission requests so the agent never waits.
    final pending = _pendingPermissions.values.toList();
    _pendingPermissions.clear();
    _cancelledPermissions.clear();
    for (final completer in pending) {
      if (!completer.isCompleted) {
        completer.complete(rt.PermissionReply.reject);
      }
    }
    _connected = false;
    // Let permission handlers emit their RPC replies before the connection
    // is torn down.
    await Future<void>.delayed(Duration.zero);
    await _connection?.close();
    _connection = null;
    if (!_updates.isClosed) {
      await _updates.close();
    }
    if (!_permissionRequests.isClosed) {
      await _permissionRequests.close();
    }
  }

  /// Handles an inbound `session/request_permission` request from the agent.
  Future<RequestPermissionResponse> _handlePermissionRequest(
    ClientContext context,
    RequestPermissionRequest request,
    RequestCancellation cancellation,
  ) {
    final requestId =
        '${request.sessionId}:${request.toolCall.toolCallId}:${_permissionCounter++}';
    final parsed = rt.PermissionRequest(
      sessionId: rt.SessionId(request.sessionId),
      requestId: requestId,
      toolCallId: request.toolCall.toolCallId,
      toolName: request.toolCall.title ?? request.toolCall.toolCallId,
      title: request.toolCall.title ?? request.toolCall.toolCallId,
      input: request.toolCall.rawInput is Map
          ? Map<String, Object?>.from(request.toolCall.rawInput as Map)
          : const <String, Object?>{},
      options: [
        for (final option in request.options)
          rt.PermissionOption(
            optionId: option.optionId,
            kind: _replyFromKind(option.kind),
            name: option.name,
          ),
      ],
    );
    _permissionRequests.add(parsed);
    return _respondPermission(requestId)
        .then((reply) {
          if (_cancelledPermissions.remove(requestId)) {
            return const RequestPermissionResponse(
              outcome: PermissionCancelled(),
            );
          }
          final kind = switch (reply) {
            rt.PermissionReply.allowOnce => PermissionOptionKind.allowOnce,
            rt.PermissionReply.allowAlways => PermissionOptionKind.allowAlways,
            rt.PermissionReply.reject => PermissionOptionKind.rejectOnce,
          };
          final option = request.options
              .where((option) => option.kind == kind)
              .firstOrNull;
          if (option != null) {
            return RequestPermissionResponse(
              outcome: PermissionSelected(optionId: option.optionId),
            );
          }
          return RequestPermissionResponse(outcome: PermissionCancelled());
        })
        .timeout(
          const Duration(minutes: 10),
          onTimeout: () {
            return const RequestPermissionResponse(
              outcome: PermissionCancelled(),
            );
          },
        );
  }

  Future<rt.PermissionReply> _respondPermission(Object requestId) {
    final completer = Completer<rt.PermissionReply>();
    _pendingPermissions[requestId] = completer;
    return completer.future;
  }

  /// Maps an ACP permission option kind to a [PermissionReply].
  static rt.PermissionReply _replyFromKind(PermissionOptionKind kind) =>
      switch (kind) {
        PermissionOptionKind.allowOnce => rt.PermissionReply.allowOnce,
        PermissionOptionKind.allowAlways => rt.PermissionReply.allowAlways,
        PermissionOptionKind.rejectOnce ||
        PermissionOptionKind.rejectAlways => rt.PermissionReply.reject,
      };

  /// Dispatches `session/update` notifications and caches session state.
  void _onUpdate(SessionUpdateNotification notification) {
    final rawSessionId = notification.sessionId;
    _updates.add(notification);
    switch (notification.update) {
      case UsageSessionUpdate(:final used, :final size):
        if (used != null) {
          _sessionUsage[rawSessionId] = used;
        }
        if (size != null && size > 0) {
          _contextSize = size;
        }
      case SessionInfoSessionUpdate(:final title):
        if (title != null && title.isNotEmpty) {
          _sessionTitles[rawSessionId] = title;
        }
      case AvailableCommandsSessionUpdate(:final update):
        _sessionCommands[rawSessionId] = List.unmodifiable([
          for (final command in update.availableCommands)
            rt.AgentCommand(
              name: command.name,
              description: command.description,
              inputHint: command.input?.hint ?? '',
            ),
        ]);
      case CurrentModeSessionUpdate(:final currentModeId):
        _sessionModes[rawSessionId] = currentModeId;
      case ConfigOptionSessionUpdate(:final configOptions):
        _applyConfigOptions(configOptions, sessionId: rawSessionId);
      default:
        break;
    }
  }

  /// The model catalog advertised by the server through config options.
  List<rt.ModelDescriptor> get catalog => _catalog;

  /// The operating modes offered by the server, if any.
  @override
  List<rt.ModeOption> get modeOptions => _modeOptions;

  /// The current mode of [sessionId], if known.
  @override
  String? modeFor(rt.SessionId sessionId) => _sessionModes[sessionId.value];

  /// Switches [sessionId] to [modeId].
  @override
  Future<void> setMode(rt.SessionId sessionId, String modeId) async {
    final response = await _client.setSessionConfigOption(
      SetValueIdConfigOption(
        sessionId: sessionId.value,
        configId: acpConfigIdMode,
        value: modeId,
      ),
    );
    _applyConfigOptions(response.configOptions, sessionId: sessionId.value);
    _sessionModes[sessionId.value] = modeId;
  }

  /// The authentication methods advertised by the server, if any.
  List<rt.AuthMethod> get authMethods => _authMethods;

  /// Authenticates with the server using [methodId].
  Future<void> authenticate(String methodId) async {
    await _client.authenticate(AuthenticateRequest(methodId: methodId));
  }

  /// Parses the `authMethods` array from an initialize response.
  static List<rt.AuthMethod> _parseAuthMethods(List<AuthMethod> raw) =>
      List.unmodifiable([
        for (final method in raw)
          switch (method) {
            AgentAuthMethod(:final agent) => rt.AuthMethod(
              id: agent.id,
              name: agent.name,
              description: agent.description ?? '',
            ),
          },
      ]);

  /// The slash commands advertised for [sessionId].
  @override
  List<rt.AgentCommand> commandsFor(rt.SessionId sessionId) =>
      _sessionCommands[sessionId.value] ?? const [];

  /// The display title reported for [sessionId], if any.
  @override
  String? titleFor(rt.SessionId sessionId) => _sessionTitles[sessionId.value];

  @override
  Stream<rt.PermissionRequest> get permissionRequests =>
      _permissionRequests.stream;

  @override
  Future<void> respondPermission(
    Object requestId,
    rt.PermissionReply reply,
  ) async {
    final completer = _pendingPermissions.remove(requestId);
    if (completer != null && !completer.isCompleted) {
      completer.complete(reply);
    }
  }

  @override
  rt.ModelRef get defaultModel => _defaultModelRef;

  @override
  Stream<rt.AgentEvent> run(rt.TurnRequest request) async* {
    final rt.SessionId sessionId;
    if (request.sessionId == null) {
      final session = await createSession(
        workingDirectory: request.workingDirectory ?? Directory.current.path,
        additionalDirectories: request.additionalDirectories ?? const [],
      );
      sessionId = session.id;
    } else {
      sessionId = request.sessionId!;
    }
    await _syncConfig(
      sessionId,
      request.model,
      request.reasoningEffort,
      request.mode,
    );
    final turnId = rt.TurnId('turn-${_turnCounter++}');
    final mapper = ClientUpdateMapper(sessionId, turnId);
    yield rt.TurnStarted(
      sessionId: sessionId,
      turnId: turnId,
      sequence: mapper.nextSequence(),
      occurredAt: DateTime.now().toUtc(),
      userMessage: rt.UserMessageItem(
        id: rt.TimelineItemId('user-${turnId.value}'),
        sessionId: sessionId,
        turnId: turnId,
        sequence: 0,
        occurredAt: DateTime.now().toUtc(),
        content: request.content,
      ),
    );
    final controller = StreamController<rt.AgentEvent>();
    final sub = _updates.stream
        .where((update) => update.sessionId == sessionId.value)
        .listen((update) {
          for (final event in mapper.map(update.update)) {
            controller.add(event);
          }
        });
    final cancellation = request.cancellation;
    void cancelTurn() {
      _cancelPendingPermissions(sessionId);
      _client.cancel(sessionId.value);
    }

    if (cancellation != null) {
      cancellation.whenCancelled.then((_) => cancelTurn());
    }
    try {
      final responseFuture = _client.prompt(
        PromptRequest(
          sessionId: sessionId.value,
          prompt: _promptBlocks(request.content),
        ),
      );
      unawaited(
        responseFuture.then(
          (response) {
            controller.add(_turnFinished(sessionId, turnId, mapper, response));
            controller.close();
          },
          onError: (Object error, StackTrace stack) {
            controller.addError(error, stack);
            controller.close();
          },
        ),
      );
      yield* controller.stream;
    } finally {
      await sub.cancel();
    }
  }

  @override
  Stream<rt.AgentEvent> compact(
    rt.SessionId sessionId, {
    String? instruction,
    rt.CancellationToken? cancellation,
  }) async* {
    final turnId = rt.TurnId('turn-${_turnCounter++}');
    final mapper = ClientUpdateMapper(sessionId, turnId);
    try {
      final result = await _connection!.connection
          .sendRequest<AtlasCompactResult>(
            atlasSessionCompactMethod,
            params: {
              'sessionId': sessionId.value,
              if (instruction != null && instruction.isNotEmpty)
                'instruction': instruction,
            },
            mapResult: (value) => AtlasCompactResult.fromJson(
              Map<String, Object?>.from(value as Map),
            ),
          );
      _sessionUsage[sessionId.value] = result.tokensAfter;
      yield rt.CompactionFinished(
        sessionId: sessionId,
        turnId: turnId,
        sequence: mapper.nextSequence(),
        occurredAt: DateTime.now().toUtc(),
        checkpoint: rt.CompactionCheckpoint(
          sessionId: sessionId,
          compactedThroughSequence: 0,
          summary: result.summaryPresent ? '[compacted]' : '',
          keptRecentMessages: result.keptMessages,
          inputTokensBefore: result.tokensBefore,
          inputTokensAfter: result.tokensAfter,
          createdAt: DateTime.now().toUtc(),
        ),
      );
      return;
    } on RpcError catch (error) {
      if (error.code != -32601) rethrow;
    }
    final controller = StreamController<rt.AgentEvent>();
    // The server reports the compact outcome as a message chunk followed by
    // a usage update; hold the message until the usage arrives so the
    // checkpoint carries the post-compaction token count.
    String? pendingCompactText;
    Map<String, Object?>? pendingCompactMeta;
    final sub = _updates.stream
        .where((update) => update.sessionId == sessionId.value)
        .listen((update) {
          switch (update.update) {
            case AgentMessageChunk(:final chunk):
              pendingCompactText = _chunkText(chunk);
              pendingCompactMeta = chunk.meta;
            case UsageSessionUpdate(:final used):
              final events = _compactEvents(
                mapper,
                pendingCompactText ?? '',
                used,
                pendingCompactMeta,
              );
              for (final event in events) {
                controller.add(event);
              }
              pendingCompactText = null;
            default:
              break;
          }
        });
    if (cancellation != null) {
      cancellation.whenCancelled.then((_) {
        _cancelPendingPermissions(sessionId);
        _client.cancel(sessionId.value);
      });
    }
    try {
      final prompt = instruction == null || instruction.isEmpty
          ? '/compact'
          : '/compact $instruction';
      final responseFuture = _client.prompt(
        PromptRequest.text(sessionId.value, prompt),
      );
      unawaited(
        responseFuture.then(
          (_) {
            if (pendingCompactText != null) {
              final events = _compactEvents(
                mapper,
                pendingCompactText!,
                _sessionUsage[sessionId.value],
                pendingCompactMeta,
              );
              for (final event in events) {
                controller.add(event);
              }
            }
            controller.close();
          },
          onError: (Object error, StackTrace stack) {
            controller.addError(error, stack);
            controller.close();
          },
        ),
      );
      yield* controller.stream;
    } finally {
      await sub.cancel();
    }
  }

  /// Resolves permission prompts belonging to a cancelled session.
  void _cancelPendingPermissions(rt.SessionId sessionId) {
    final prefix = '${sessionId.value}:';
    for (final entry in _pendingPermissions.entries.toList()) {
      if (entry.key is String && (entry.key as String).startsWith(prefix)) {
        _cancelledPermissions.add(entry.key);
        if (!entry.value.isCompleted) {
          entry.value.complete(rt.PermissionReply.reject);
        }
        _pendingPermissions.remove(entry.key);
      }
    }
  }

  @override
  Future<rt.SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  }) async {
    final response = await _client.listSessions(
      ListSessionsRequest(cwd: workingDirectory, cursor: cursor),
    );
    final items = <rt.SessionSummary>[];
    for (final entry in response.sessions) {
      _sessionCwd[entry.sessionId] = entry.cwd;
      items.add(
        rt.SessionSummary(
          id: rt.SessionId(entry.sessionId),
          title: entry.title ?? '',
          workingDirectory: entry.cwd,
          updatedAt: _parseTime(entry.updatedAt),
        ),
      );
    }
    return rt.SessionPage(items: items, nextCursor: response.nextCursor);
  }

  @override
  Future<rt.Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const <String>[],
  }) async {
    final response = await _client.newSession(
      NewSessionRequest(
        cwd: workingDirectory,
        mcpServers: const [],
        additionalDirectories: additionalDirectories,
      ),
    );
    final id = rt.SessionId(response.sessionId);
    _sessionCwd[id.value] = workingDirectory;
    _sessionModels[id.value] = defaultModel;
    _applyConfigOptions(response.configOptions, sessionId: id.value);
    return rt.Session(
      id: id,
      workingDirectory: workingDirectory,
      additionalDirectories: List<String>.unmodifiable(additionalDirectories),
      createdAt: DateTime.now().toUtc(),
      updatedAt: DateTime.now().toUtc(),
    );
  }

  @override
  Future<rt.SessionSnapshot> loadSession(rt.SessionId sessionId) async {
    final cwd = _sessionCwd[sessionId.value] ?? await _findCwd(sessionId);
    final mapper = ClientTimelineMapper(sessionId);
    final timeline = <rt.TimelineItem>[];
    final sub = _updates.stream
        .where((update) => update.sessionId == sessionId.value)
        .listen((update) {
          timeline.addAll(mapper.map(update.update));
        });
    try {
      final response = await _client.loadSession(
        LoadSessionRequest(
          sessionId: sessionId.value,
          cwd: cwd,
          mcpServers: const [],
        ),
      );
      _applyConfigOptions(response.configOptions, sessionId: sessionId.value);
      return rt.SessionSnapshot(
        session: rt.Session(
          id: sessionId,
          workingDirectory: cwd,
          createdAt: DateTime.now().toUtc(),
          updatedAt: DateTime.now().toUtc(),
        ),
        turns: const [],
        timeline: timeline,
      );
    } finally {
      await sub.cancel();
    }
  }

  @override
  Future<void> deleteSession(rt.SessionId sessionId) async {
    try {
      await _client.deleteSession(
        DeleteSessionRequest(sessionId: sessionId.value),
      );
    } on RpcError catch (error) {
      // Some servers (OpenCode) implement `session/close` instead of
      // `session/delete`; fall back so deletion still works there.
      if (error.code != -32601) {
        rethrow;
      }
      await closeSession(sessionId);
    }
    _sessionCwd.remove(sessionId.value);
    _sessionModels.remove(sessionId.value);
    _sessionEfforts.remove(sessionId.value);
    _sessionUsage.remove(sessionId.value);
  }

  /// Closes an active session, cancelling its work and freeing resources.
  Future<void> closeSession(rt.SessionId sessionId) async {
    await _client.closeSession(CloseSessionRequest(sessionId: sessionId.value));
    _sessionCwd.remove(sessionId.value);
    _sessionModels.remove(sessionId.value);
    _sessionEfforts.remove(sessionId.value);
    _sessionUsage.remove(sessionId.value);
  }

  @override
  Future<void> renameSession(rt.SessionId sessionId, String title) async {
    final trimmed = title.trim();
    if (trimmed.isEmpty) {
      throw ArgumentError('title must not be empty');
    }
    try {
      await _connection!.connection.sendRequest<void>(
        acpSessionSetTitleMethod,
        params: {'sessionId': sessionId.value, 'title': trimmed},
        mapResult: (_) {},
      );
    } on RpcError catch (error) {
      // Third-party agents have no rename method; keep a local overlay so
      // the Flutter sidebar still shows the user's title.
      if (error.code != -32601) {
        rethrow;
      }
    }
    _sessionTitles[sessionId.value] = trimmed;
  }

  @override
  Future<int> contextWindowSize() async => _contextSize;

  /// Ensures the server session uses [model], [effort], and [mode], sending
  /// `session/set_config_option` when they differ from the last known values.
  Future<void> _syncConfig(
    rt.SessionId sessionId,
    rt.ModelRef? model,
    String? effort,
    String? mode,
  ) async {
    final currentModel = _sessionModels[sessionId.value] ?? defaultModel;
    final targetModel = model ?? currentModel;
    if (targetModel != currentModel) {
      final response = await _client.setSessionConfigOption(
        SetValueIdConfigOption(
          sessionId: sessionId.value,
          configId: acpConfigIdModel,
          value: targetModel.toString(),
        ),
      );
      _applyConfigOptions(response.configOptions, sessionId: sessionId.value);
      _sessionModels[sessionId.value] = targetModel;
    }
    final currentEffort = _sessionEfforts[sessionId.value];
    if (effort != null && effort != currentEffort) {
      final response = await _client.setSessionConfigOption(
        SetValueIdConfigOption(
          sessionId: sessionId.value,
          configId: _effortConfigId,
          value: effort,
        ),
      );
      _applyConfigOptions(response.configOptions, sessionId: sessionId.value);
      _sessionEfforts[sessionId.value] = effort;
    }
    final currentMode = _sessionModes[sessionId.value];
    if (mode != null && mode != currentMode) {
      final response = await _client.setSessionConfigOption(
        SetValueIdConfigOption(
          sessionId: sessionId.value,
          configId: acpConfigIdMode,
          value: mode,
        ),
      );
      _applyConfigOptions(response.configOptions, sessionId: sessionId.value);
      _sessionModes[sessionId.value] = mode;
    }
  }

  /// Parses the server `configOptions` payload and updates the model catalog.
  ///
  /// [sessionId] scopes session-level state such as the current mode.
  void _applyConfigOptions(List<SessionConfigOption> raw, {String? sessionId}) {
    var currentModel = _defaultModelRef;
    for (final entry in raw) {
      if (entry.id == acpConfigIdModel &&
          entry is SessionConfigSelectOptionValue) {
        currentModel = _modelRefFromValue(entry.currentValue);
        final options = switch (entry.options) {
          SessionConfigUngroupedOptions(:final options) => options,
          SessionConfigGroupedOptions(:final groups) => [
            for (final group in groups) ...group.options,
          ],
        };
        final catalog = <rt.ModelDescriptor>[];
        for (final option in options) {
          final value = option.value;
          if (value.isEmpty) {
            continue;
          }
          final ref = _modelRefFromValue(value);
          catalog.add(
            rt.ModelDescriptor(
              ref: ref,
              name: option.name,
              description: option.description ?? '',
              reasoningEfforts: _effortsByModel[ref] ?? const [],
            ),
          );
        }
        if (catalog.isNotEmpty) {
          _catalog = List.unmodifiable(catalog);
        }
      } else if (entry.id == acpConfigIdEffort &&
          entry is SessionConfigSelectOptionValue) {
        // OpenCode names the option `effort`; remember the server's id so
        // set_config_option targets the right one.
        _effortConfigId = entry.id;
        final options = switch (entry.options) {
          SessionConfigUngroupedOptions(:final options) => options,
          SessionConfigGroupedOptions(:final groups) => [
            for (final group in groups) ...group.options,
          ],
        };
        final efforts = <rt.ReasoningEffortOption>[
          for (final option in options)
            rt.ReasoningEffortOption(
              value: option.value,
              name: option.name,
              description: option.description ?? '',
            ),
        ];
        _effortsByModel[currentModel] = List.unmodifiable(
          efforts.where((effort) => effort.value.isNotEmpty),
        );
        _rebuildCatalogEfforts();
      } else if (entry.id == acpConfigIdMode &&
          entry is SessionConfigSelectOptionValue) {
        if (sessionId != null && entry.currentValue.isNotEmpty) {
          _sessionModes[sessionId] = entry.currentValue;
        }
        final options = switch (entry.options) {
          SessionConfigUngroupedOptions(:final options) => options,
          SessionConfigGroupedOptions(:final groups) => [
            for (final group in groups) ...group.options,
          ],
        };
        _modeOptions = List.unmodifiable([
          for (final option in options)
            rt.ModeOption(
              id: option.value,
              name: option.name,
              description: option.description ?? '',
            ),
        ]);
      }
    }
    _defaultModelRef = currentModel;
  }

  /// Rebuilds [catalog] with the cached efforts per model.
  void _rebuildCatalogEfforts() {
    _catalog = List.unmodifiable([
      for (final model in _catalog)
        rt.ModelDescriptor(
          ref: model.ref,
          name: model.name,
          description: model.description,
          reasoningEfforts: _effortsByModel[model.ref] ?? const [],
        ),
    ]);
  }

  /// Parses a `<provider>/<model>` value into a model reference.
  static rt.ModelRef _modelRefFromValue(String value) {
    final slash = value.indexOf('/');
    if (slash > 0 && slash < value.length - 1) {
      return rt.ModelRef(
        providerId: rt.ProviderId(value.substring(0, slash)),
        modelId: rt.ModelId(value.substring(slash + 1)),
      );
    }
    return rt.ModelRef(
      providerId: rt.ProviderId('acp'),
      modelId: rt.ModelId(value),
    );
  }

  /// Finds the working directory of [sessionId] through `session/list`.
  Future<String> _findCwd(rt.SessionId sessionId) async {
    final page = await listSessions();
    for (final summary in page.items) {
      if (summary.id == sessionId) {
        return summary.workingDirectory;
      }
    }
    return Directory.current.path;
  }

  /// Builds the ACP prompt blocks for [content].
  List<ContentBlock> _promptBlocks(List<rt.ContentPart> content) => [
    for (final part in content)
      switch (part) {
        rt.TextContent(:final text) => TextContentBlock(text: text),
        rt.ImageContent(:final source, :final mimeType) => ImageContent(
          data: _imageData(source),
          mimeType: mimeType ?? 'image/png',
        ),
        rt.ResourceContent(:final uri, :final text) => EmbeddedResource(
          resource: TextResourceContents(uri: uri, text: text),
        ),
      },
  ];

  /// Extracts the base64 payload from an image data URL, or passes the
  /// source through when it is not a data URL.
  static String _imageData(String source) {
    const prefix = ';base64,';
    final index = source.indexOf(prefix);
    return index < 0 ? source : source.substring(index + prefix.length);
  }

  /// Converts a `session/prompt` response into a terminal turn event.
  rt.TurnFinished _turnFinished(
    rt.SessionId sessionId,
    rt.TurnId turnId,
    ClientUpdateMapper mapper,
    PromptResponse response,
  ) {
    final stopReason = response.stopReason;
    final status = switch (stopReason) {
      StopReason.cancelled => rt.TurnStatus.cancelled,
      _ => rt.TurnStatus.completed,
    };
    return rt.TurnFinished(
      sessionId: sessionId,
      turnId: turnId,
      sequence: mapper.nextSequence(),
      occurredAt: DateTime.now().toUtc(),
      outcome: rt.TurnOutcome(
        sessionId: sessionId,
        turnId: turnId,
        status: status,
        stopReason: stopReason == StopReason.maxTokens
            ? rt.StopReason.maxTokens
            : rt.StopReason.endTurn,
        usage: rt.TokenUsage(
          inputTokens: _sessionUsage[sessionId.value] ?? 0,
          totalTokens: _sessionUsage[sessionId.value] ?? 0,
        ),
      ),
    );
  }

  /// Converts a compact outcome message into runtime compaction events.
  List<rt.AgentEvent> _compactEvents(
    ClientUpdateMapper mapper,
    String text,
    int? usedTokens,
    Map<String, Object?>? meta,
  ) {
    final atlas = meta?['atlas.dev'];
    final compact = atlas is Map ? atlas['compact'] : null;
    if (compact is Map && compact['status'] == 'completed') {
      final kept = compact['keptMessages'];
      return [
        rt.CompactionFinished(
          sessionId: mapper.sessionId,
          turnId: mapper.turnId,
          sequence: mapper.nextSequence(),
          occurredAt: DateTime.now().toUtc(),
          checkpoint: rt.CompactionCheckpoint(
            sessionId: mapper.sessionId,
            compactedThroughSequence: 0,
            summary: '',
            keptRecentMessages: kept is num ? kept.toInt() : 0,
            inputTokensBefore: 0,
            inputTokensAfter: usedTokens ?? 0,
            createdAt: DateTime.now().toUtc(),
          ),
        ),
      ];
    }
    if (compact is Map && compact['status'] == 'failed') {
      return [
        rt.CompactionFailed(
          sessionId: mapper.sessionId,
          turnId: mapper.turnId,
          sequence: mapper.nextSequence(),
          occurredAt: DateTime.now().toUtc(),
          message: 'Compaction failed.',
        ),
      ];
    }
    return const [];
  }

  /// The text payload of an `agent_message_chunk` update.
  static String _chunkText(ContentChunk chunk) {
    final content = chunk.content;
    if (content is TextContentBlock) {
      return content.text;
    }
    return '';
  }

  static DateTime _parseTime(Object? value) {
    final parsed = value is String ? DateTime.tryParse(value) : null;
    return parsed?.toLocal() ?? DateTime.now();
  }
}

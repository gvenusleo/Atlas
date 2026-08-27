import 'dart:async';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/runtime_environment.dart';
import 'workspace_message.dart';
import 'workspace_state.dart';

/// Working directory for the workspace, overridable in tests.
final workspaceWorkingDirectoryProvider =
    NotifierProvider<WorkspaceWorkingDirectory, String>(
      WorkspaceWorkingDirectory.new,
    );

/// Holds the workspace working directory.
///
/// Cold start uses the user home directory so a packaged app does not inherit
/// the process current directory (`/` when launched from Finder).
class WorkspaceWorkingDirectory extends Notifier<String> {
  @override
  String build() => _homeDirectory() ?? Directory.current.path;

  /// Switches the working directory for subsequent sessions.
  void set(String directory) => state = directory;
}

String? _homeDirectory() {
  final home =
      Platform.environment['HOME'] ?? Platform.environment['USERPROFILE'];
  if (home == null || home.isEmpty) {
    return null;
  }
  return home;
}

/// Coordinates one Flutter workspace with the injected shared runtime.
final class WorkspaceController extends Notifier<WorkspaceState> {
  final _cancellations = <String, CancellationToken>{};
  final _streamOpen = <String, bool>{};
  final _recentKeys = <String>[];
  StreamSubscription<PermissionRequest>? _permissionSub;
  var _localId = 0;
  var _draftSerial = 0;

  /// Idle session caches kept besides the focused and running ones.
  static const _maxIdleWorkspaces = 8;

  /// Runtime services supplied by the application composition root.
  RuntimeEnvironment get _environment =>
      _environmentCache ??
      (throw StateError('workspaceProvider requires a composed runtime'));

  RuntimeEnvironment? _environmentCache;

  /// Models available in the current configuration.
  List<ModelDescriptor> get models => _environment.models;

  @override
  WorkspaceState build() {
    // Read once instead of watching: a runtime switch must reset session
    // caches through the listen callback, not rebuild the controller (which
    // would lose the callback registration).
    final runtimeState = ref.read(runtimeEnvironmentProvider);
    _environmentCache = runtimeState.environment;
    if (_environmentCache == null) {
      throw StateError('workspaceProvider requires a composed runtime');
    }
    ref.onDispose(_cancelAll);
    // A runtime switch (ACP activation) replaces sessions and caches: the
    // new runtime owns a different session list.
    ref.listen(runtimeEnvironmentProvider, (previous, next) {
      _environmentCache = next.environment;
      if (_environmentCache != null) {
        _resetForRuntime();
      }
    });
    _subscribePermissions();
    final draftKey = _nextDraftKey();
    _touch(draftKey);
    return WorkspaceState(
      activeKey: draftKey,
      workspaces: {
        draftKey: _draftWorkspace(ref.read(workspaceWorkingDirectoryProvider)),
      },
      sessions: const [],
    );
  }

  /// Discards session caches and starts a fresh draft after a runtime switch.
  void _resetForRuntime() {
    _cancellations.clear();
    _streamOpen.clear();
    _recentKeys.clear();
    _subscribePermissions();
    final draftKey = _nextDraftKey();
    _touch(draftKey);
    state = WorkspaceState(
      activeKey: draftKey,
      workspaces: {
        draftKey: _draftWorkspace(ref.read(workspaceWorkingDirectoryProvider)),
      },
      sessions: const [],
    );
    unawaited(refreshSessions(showLoading: false));
  }

  /// Subscribes to permission requests from the current runtime.
  ///
  /// Local runtimes execute tools directly and expose no permission port;
  /// remote agents surface requests that must be answered by the user.
  void _subscribePermissions() {
    _permissionSub?.cancel();
    _permissionSub = null;
    final runtime = _environment.runtime;
    if (runtime is! PermissionPort) {
      return;
    }
    final port = runtime as PermissionPort;
    _permissionSub = port.permissionRequests.listen((request) {
      final pending = state.pendingPermissions;
      if (pending.any((item) => item.requestId == request.requestId)) {
        return;
      }
      state = state.copyWith(pendingPermissions: [...pending, request]);
    });
  }

  /// Replies to a pending permission request with the user's decision.
  Future<void> respondPermission(
    Object requestId,
    PermissionReply reply,
  ) async {
    final runtime = _environment.runtime;
    if (runtime is! PermissionPort) {
      return;
    }
    final port = runtime as PermissionPort;
    await port.respondPermission(requestId, reply);
    state = state.copyWith(
      pendingPermissions: state.pendingPermissions
          .where((request) => request.requestId != requestId)
          .toList(),
    );
  }

  Future<void>? _refreshInFlight;

  /// Loads every persisted session across all directories, newest first.
  Future<void> refreshSessions({bool showLoading = true}) {
    return _refreshInFlight ??= _refreshSessions(
      showLoading: showLoading,
    ).whenComplete(() => _refreshInFlight = null);
  }

  Future<void> _refreshSessions({required bool showLoading}) async {
    if (showLoading) {
      state = state.copyWith(loadingSessions: true);
    }
    try {
      final sessions = <SessionSummary>[];
      String? cursor;
      do {
        final page = await _environment.runtime.listSessions(
          cursor: cursor,
          limit: 100,
        );
        sessions.addAll(page.items);
        cursor = page.nextCursor;
      } while (cursor != null && sessions.length < 500);
      // Merge titles reported by the agent through session_info_update.
      final runtime = _environment.runtime;
      for (var i = 0; i < sessions.length; i++) {
        final title = runtime.titleFor(sessions[i].id);
        if (title != null && title.isNotEmpty) {
          sessions[i] = SessionSummary(
            id: sessions[i].id,
            title: title,
            workingDirectory: sessions[i].workingDirectory,
            updatedAt: sessions[i].updatedAt,
          );
        }
      }
      state = state.copyWith(sessions: sessions, loadingSessions: false);
    } catch (error) {
      _append(
        state.activeKey,
        WorkspaceMessageKind.error,
        'Cannot load sessions: $error',
      );
      state = state.copyWith(loadingSessions: false);
    }
  }

  /// Focuses a new draft so the next prompt creates a new session.
  ///
  /// Running sessions keep their cached transcripts and continue in the
  /// background. [workingDirectory] defaults to the focused session's directory.
  void newSession({String? workingDirectory}) {
    final directory = workingDirectory ?? state.workingDirectory;
    if (state.sessionId == null &&
        state.messages.isEmpty &&
        !state.busy &&
        state.workingDirectory == directory) {
      return;
    }
    if (workingDirectory != null) {
      ref.read(workspaceWorkingDirectoryProvider.notifier).set(directory);
    }
    final draftKey = _nextDraftKey();
    final workspaces = Map<String, SessionWorkspace>.from(state.workspaces);
    workspaces[draftKey] = _draftWorkspace(
      directory,
      model: state.activeModel,
      reasoningEffort: state.reasoningEffort,
    );
    _touch(draftKey);
    _evictIdle(workspaces, keepKey: draftKey);
    state = state.copyWith(activeKey: draftKey, workspaces: workspaces);
  }

  /// Loads a persisted session, or focuses it when its cache is already warm.
  Future<void> resume(SessionId id) async {
    final cachedKey = _keyForSession(id);
    if (cachedKey != null) {
      final cached = state.workspaces[cachedKey]!;
      _focus(cachedKey, cached.workingDirectory);
      return;
    }
    try {
      final snapshot = await _environment.runtime.loadSession(id);
      final selection = _selectionFromTurns(snapshot.turns);
      final workspace = SessionWorkspace(
        sessionId: snapshot.session.id,
        workingDirectory: snapshot.session.workingDirectory,
        messages: _messagesFromTimeline(snapshot.timeline),
        contextTokens: snapshot.session.lastUsage.totalTokens,
        hasImages: _timelineHasImages(snapshot.timeline),
        activeModel: selection.model,
        reasoningEffort: selection.effort,
        mode: _environment.runtime.modeFor(id),
      );
      final workspaces = Map<String, SessionWorkspace>.from(state.workspaces);
      workspaces[id.value] = workspace;
      _streamOpen[id.value] = false;
      _touch(id.value);
      _evictIdle(workspaces, keepKey: id.value);
      state = state.copyWith(activeKey: id.value, workspaces: workspaces);
      ref
          .read(workspaceWorkingDirectoryProvider.notifier)
          .set(snapshot.session.workingDirectory);
    } catch (error) {
      _append(
        state.activeKey,
        WorkspaceMessageKind.error,
        'Cannot resume session: $error',
      );
    }
  }

  /// Renames a persisted session's display title.
  Future<void> renameSession(SessionId id, String title) async {
    try {
      await _environment.runtime.renameSession(id, title);
      await refreshSessions();
    } catch (error) {
      _append(
        state.activeKey,
        WorkspaceMessageKind.error,
        'Cannot rename session: $error',
      );
    }
  }

  /// Deletes a persisted session, cancelling it when a turn is in flight.
  Future<void> deleteSession(SessionId id) async {
    final key = _keyForSession(id) ?? id.value;
    _cancellations.remove(key)?.cancel();
    _streamOpen.remove(key);
    _recentKeys.remove(key);
    try {
      await _environment.runtime.deleteSession(id);
      final workspaces = Map<String, SessionWorkspace>.from(state.workspaces)
        ..remove(key);
      var activeKey = state.activeKey;
      if (activeKey == key) {
        activeKey = _ensureDraft(workspaces, state.workingDirectory);
      }
      state = state.copyWith(activeKey: activeKey, workspaces: workspaces);
      await refreshSessions();
    } catch (error) {
      _append(
        state.activeKey,
        WorkspaceMessageKind.error,
        'Cannot delete session: $error',
      );
    }
  }

  /// Changes a session's model and resets reasoning effort.
  ///
  /// Defaults to the focused session when [sessionKey] is omitted.
  void selectModel(ModelDescriptor model, {String? sessionKey}) {
    final key = sessionKey ?? state.activeKey;
    _patch(
      key,
      (workspace) => workspace.copyWith(
        activeModel: model,
        reasoningEffort: model.reasoningEfforts.firstOrNull?.value,
      ),
    );
    final workspace = state.workspaces[key];
    if (workspace != null &&
        workspace.hasImages &&
        !model.inputCapabilities.contains(ModelInputCapability.image)) {
      final label = model.name.isEmpty ? model.ref.modelId.value : model.name;
      _append(
        key,
        WorkspaceMessageKind.notice,
        '$label does not support images; images in this conversation will be omitted.',
      );
    }
  }

  /// Changes a session's reasoning effort for subsequent turns.
  ///
  /// Defaults to the focused session when [sessionKey] is omitted.
  void selectReasoningEffort(String? effort, {String? sessionKey}) {
    _patch(
      sessionKey ?? state.activeKey,
      (workspace) => workspace.copyWith(reasoningEffort: effort),
    );
  }

  /// Changes a session's agent mode for subsequent turns.
  ///
  /// Applies immediately when the session already exists; drafts carry the
  /// selection until the first turn creates the session. Defaults to the
  /// focused session when [sessionKey] is omitted.
  Future<void> selectMode(String mode, {String? sessionKey}) async {
    final key = sessionKey ?? state.activeKey;
    _patch(key, (workspace) => workspace.copyWith(mode: mode));
    final sessionId = state.workspaces[key]?.sessionId;
    if (sessionId != null) {
      try {
        await _environment.runtime.setMode(sessionId, mode);
      } catch (error) {
        _append(key, WorkspaceMessageKind.error, 'Cannot set mode: $error');
      }
    }
  }

  /// Appends a local notice to a session transcript.
  ///
  /// Defaults to the focused session when [sessionKey] is omitted.
  void notify(String text, {String? sessionKey}) {
    _append(sessionKey ?? state.activeKey, WorkspaceMessageKind.notice, text);
  }

  /// Shows the terminal or file browser for the focused session.
  void setShowTerminal(bool showTerminal) {
    _patch(
      state.activeKey,
      (workspace) => workspace.copyWith(showTerminal: showTerminal),
    );
  }

  /// Submits a prompt or executes a TUI-compatible slash command.
  ///
  /// Defaults to the focused session when [sessionKey] is omitted.
  /// Returns false when the prompt is empty, the session is busy, or the
  /// prompt is rejected before a turn starts.
  Future<bool> send(
    String rawText, {
    String? sessionKey,
    List<ImageContent> images = const [],
  }) async {
    var key = sessionKey ?? state.activeKey;
    final workspace = state.workspaces[key];
    final text = rawText.trim();
    if ((text.isEmpty && images.isEmpty) ||
        workspace == null ||
        workspace.busy) {
      return false;
    }
    if (text.startsWith('/')) {
      if (images.isNotEmpty) {
        _append(
          key,
          WorkspaceMessageKind.notice,
          'Slash commands do not support images.',
        );
        return false;
      }
      if (await _handleSlashCommand(text, sessionKey: key)) {
        return true;
      }
    }
    if (images.isNotEmpty &&
        !workspace.activeModel.inputCapabilities.contains(
          ModelInputCapability.image,
        )) {
      final label = workspace.activeModel.name.isEmpty
          ? workspace.activeModel.ref.modelId.value
          : workspace.activeModel.name;
      _append(
        key,
        WorkspaceMessageKind.notice,
        '$label does not support image input.',
      );
      return false;
    }
    final content = <ContentPart>[
      if (text.isNotEmpty) TextContent(text),
      ...images,
    ];
    final cancellation = CancellationToken();
    _cancellations[key] = cancellation;
    _streamOpen[key] = false;
    _patch(
      key,
      (workspace) => workspace.copyWith(
        busy: true,
        turnPhase: TurnPhase.working,
        turnStartedAt: DateTime.now(),
        hasImages: workspace.hasImages || images.isNotEmpty,
      ),
    );
    _append(
      key,
      WorkspaceMessageKind.user,
      text,
      imageSources: [for (final image in images) image.source],
    );
    try {
      await for (final event in _environment.runtime.run(
        TurnRequest(
          content: content,
          sessionId: state.workspaces[key]?.sessionId,
          workingDirectory:
              state.workspaces[key]?.workingDirectory ?? state.workingDirectory,
          model: (state.workspaces[key] ?? state.active).activeModel.ref,
          reasoningEffort:
              (state.workspaces[key] ?? state.active).reasoningEffort,
          mode: (state.workspaces[key] ?? state.active).mode,
          skills: _selectedSkills(text),
          cancellation: cancellation,
        ),
      )) {
        key = _applyEvent(key, event);
      }
    } on TurnCancelledException {
      _append(key, WorkspaceMessageKind.notice, 'Turn cancelled');
    } catch (error) {
      _append(key, WorkspaceMessageKind.error, 'Turn failed: $error');
    } finally {
      _cancellations.remove(key);
      _streamOpen[key] = false;
      _patch(
        key,
        (workspace) => workspace.copyWith(
          busy: false,
          turnPhase: TurnPhase.idle,
          turnStartedAt: null,
          hasCompletedTurn: state.activeKey != key,
        ),
      );
      await refreshSessions(showLoading: false);
    }
    return true;
  }

  /// Cancels a session's runtime operation.
  ///
  /// Defaults to the focused session when [sessionKey] is omitted.
  void cancel({String? sessionKey}) =>
      _cancellations[sessionKey ?? state.activeKey]?.cancel();

  Future<bool> _handleSlashCommand(String text, {String? sessionKey}) async {
    final key = sessionKey ?? state.activeKey;
    final parts = text.split(RegExp(r'\s+'));
    switch (parts.first) {
      case '/compact':
        await _compact(parts.skip(1).join(' '), sessionKey: key);
        return true;
      default:
        return false;
    }
  }

  Future<void> _compact(String instruction, {String? sessionKey}) async {
    var key = sessionKey ?? state.activeKey;
    final workspace = state.workspaces[key];
    final id = workspace?.sessionId;
    if (id == null || workspace == null || workspace.busy) {
      _append(key, WorkspaceMessageKind.notice, 'No session to compact.');
      return;
    }
    final cancellation = CancellationToken();
    _cancellations[key] = cancellation;
    _patch(
      key,
      (workspace) => workspace.copyWith(
        busy: true,
        turnPhase: TurnPhase.compacting,
        turnStartedAt: DateTime.now(),
      ),
    );
    try {
      await for (final event in _environment.runtime.compact(
        id,
        instruction: instruction.isEmpty ? null : instruction,
        cancellation: cancellation,
      )) {
        key = _applyEvent(key, event);
      }
    } catch (error) {
      _append(key, WorkspaceMessageKind.error, 'Compaction failed: $error');
    } finally {
      _cancellations.remove(key);
      _patch(
        key,
        (workspace) => workspace.copyWith(
          busy: false,
          turnPhase: TurnPhase.idle,
          turnStartedAt: null,
          hasCompletedTurn: state.activeKey != key,
        ),
      );
    }
  }

  String _applyEvent(String key, AgentEvent event) {
    var target = _keyForSession(event.sessionId) ?? key;
    if (event is TurnStarted) {
      unawaited(refreshSessions(showLoading: false));
    }
    switch (event) {
      case TurnStarted():
        _patch(
          target,
          (workspace) => workspace.copyWith(
            sessionId: event.sessionId,
            turnPhase: TurnPhase.working,
            turnStartedAt: workspace.turnStartedAt ?? DateTime.now(),
          ),
        );
      case ModelTextDelta(:final delta):
        _finishRunningReasoning(target);
        _appendDelta(target, WorkspaceMessageKind.assistant, delta);
        _patch(
          target,
          (workspace) => workspace.copyWith(turnPhase: TurnPhase.working),
        );
      case ModelReasoningDelta(:final delta):
        _appendDelta(target, WorkspaceMessageKind.reasoning, delta);
        _patch(
          target,
          (workspace) => workspace.copyWith(turnPhase: TurnPhase.thinking),
        );
      case PlanUpdated(:final entries):
        _finishRunningReasoning(target);
        _patch(target, (workspace) {
          final planText = entries
              .map((entry) => '${_planMarker(entry.status)} ${entry.content}')
              .join('\n');
          final index = workspace.messages.lastIndexWhere(
            (message) => message.kind == WorkspaceMessageKind.plan,
          );
          final messages = [...workspace.messages];
          if (index < 0) {
            messages.add(
              WorkspaceMessage(
                id: 'plan-${event.turnId.value}',
                kind: WorkspaceMessageKind.plan,
                text: planText,
              ),
            );
          } else {
            messages[index] = messages[index].copyWith(text: planText);
          }
          return workspace.copyWith(
            turnPhase: TurnPhase.working,
            messages: messages,
          );
        });
      case ToolStarted(:final call):
        _finishRunningReasoning(target);
        _streamOpen[target] = false;
        _patch(target, (workspace) {
          return workspace.copyWith(
            turnPhase: TurnPhase.working,
            messages: [
              ...workspace.messages,
              WorkspaceMessage(
                id: call.call.id.value,
                kind: WorkspaceMessageKind.tool,
                text: '',
                toolName: call.call.name,
                arguments: call.call.arguments,
                startedAt: DateTime.now(),
                isRunning: true,
              ),
            ],
          );
        });
      case ToolFinished(:final result):
        _streamOpen[target] = false;
        _patch(target, (workspace) {
          final index = workspace.messages.indexWhere(
            (message) =>
                message.kind == WorkspaceMessageKind.tool &&
                message.isRunning &&
                message.id == result.callId.value,
          );
          if (index < 0) {
            return workspace.copyWith(turnPhase: TurnPhase.working);
          }
          final messages = [...workspace.messages];
          messages[index] = messages[index].copyWith(
            text: result.content,
            isError: result.isError,
            isRunning: false,
          );
          return workspace.copyWith(
            turnPhase: TurnPhase.working,
            messages: messages,
          );
        });
      case TurnFinished(:final outcome):
        _finishRunningReasoning(target);
        _streamOpen[target] = false;
        if (outcome.status == TurnStatus.cancelled) {
          _append(target, WorkspaceMessageKind.notice, 'Turn cancelled');
        } else if (outcome.failure != null) {
          _append(target, WorkspaceMessageKind.error, outcome.failure!.message);
        }
        _patch(
          target,
          (workspace) => workspace.copyWith(
            turnPhase: TurnPhase.idle,
            contextTokens: outcome.usage.inputTokens > 0
                ? outcome.usage.inputTokens
                : outcome.usage.totalTokens,
          ),
        );
      case CompactionStarted():
        _patch(
          target,
          (workspace) => workspace.copyWith(
            turnPhase: TurnPhase.compacting,
            turnStartedAt: workspace.turnStartedAt ?? DateTime.now(),
          ),
        );
      case CompactionFinished(:final checkpoint):
        _patch(
          target,
          (workspace) => workspace.copyWith(
            turnPhase: TurnPhase.idle,
            contextTokens: checkpoint.inputTokensAfter,
          ),
        );
        _append(
          target,
          WorkspaceMessageKind.notice,
          'Context compacted, kept ${checkpoint.keptRecentMessages} recent messages.',
        );
      case CompactionFailed(:final message):
        _patch(
          target,
          (workspace) => workspace.copyWith(turnPhase: TurnPhase.idle),
        );
        _append(
          target,
          WorkspaceMessageKind.error,
          'Compaction failed: $message',
        );
      default:
        break;
    }
    return target;
  }

  void _appendDelta(String key, WorkspaceMessageKind kind, String delta) {
    _patch(key, (workspace) {
      final messages = workspace.messages;
      final last = messages.lastOrNull;
      if ((_streamOpen[key] ?? false) && last?.kind == kind) {
        return workspace.copyWith(
          messages: [
            ...messages.sublist(0, messages.length - 1),
            last!.copyWith(text: last.text + delta),
          ],
        );
      }
      return workspace.copyWith(
        messages: [
          ...messages,
          WorkspaceMessage(
            id: _nextId(),
            kind: kind,
            text: delta,
            startedAt: kind == WorkspaceMessageKind.reasoning
                ? DateTime.now()
                : null,
            isRunning: kind == WorkspaceMessageKind.reasoning,
          ),
        ],
      );
    });
    _streamOpen[key] = true;
  }

  void _append(
    String key,
    WorkspaceMessageKind kind,
    String text, {
    List<String> imageSources = const [],
  }) {
    _streamOpen[key] = false;
    _patch(key, (workspace) {
      return workspace.copyWith(
        messages: [
          ...workspace.messages,
          WorkspaceMessage(
            id: _nextId(),
            kind: kind,
            text: text,
            imageSources: imageSources,
          ),
        ],
      );
    });
  }

  /// Marks the latest streaming reasoning item complete.
  void _finishRunningReasoning(String key) {
    _patch(key, (workspace) {
      final index = workspace.messages.lastIndexWhere(
        (message) =>
            message.kind == WorkspaceMessageKind.reasoning && message.isRunning,
      );
      if (index < 0) {
        return workspace;
      }
      final messages = [...workspace.messages];
      messages[index] = messages[index].copyWith(isRunning: false);
      return workspace.copyWith(messages: messages);
    });
  }

  void _patch(
    String key,
    SessionWorkspace Function(SessionWorkspace workspace) update,
  ) {
    final current = state.workspaces[key];
    if (current == null) {
      return;
    }
    final workspaces = Map<String, SessionWorkspace>.from(state.workspaces);
    workspaces[key] = update(current);
    state = state.copyWith(workspaces: workspaces);
  }

  String? _keyForSession(SessionId id) {
    for (final entry in state.workspaces.entries) {
      if (entry.value.sessionId == id || entry.key == id.value) {
        return entry.key;
      }
    }
    return null;
  }

  void _focus(String key, String workingDirectory) {
    _patch(key, (workspace) => workspace.copyWith(hasCompletedTurn: false));
    _touch(key);
    final workspaces = Map<String, SessionWorkspace>.from(state.workspaces);
    _evictIdle(workspaces, keepKey: key);
    state = state.copyWith(activeKey: key, workspaces: workspaces);
    ref.read(workspaceWorkingDirectoryProvider.notifier).set(workingDirectory);
  }

  void _touch(String key) {
    _recentKeys.remove(key);
    _recentKeys.add(key);
  }

  void _evictIdle(
    Map<String, SessionWorkspace> workspaces, {
    required String keepKey,
  }) {
    final seen = <String>{};
    final idle = [
      for (final key in [..._recentKeys, ...workspaces.keys])
        if (seen.add(key) &&
            key != keepKey &&
            workspaces[key] != null &&
            !(workspaces[key]!.busy))
          key,
    ];
    idle.sort((a, b) {
      final emptyA = _isEmptyDraft(workspaces[a]!);
      final emptyB = _isEmptyDraft(workspaces[b]!);
      if (emptyA != emptyB) {
        return emptyA ? -1 : 1;
      }
      return 0;
    });
    while (idle.length > _maxIdleWorkspaces) {
      final evicted = idle.removeAt(0);
      workspaces.remove(evicted);
      _recentKeys.remove(evicted);
      _streamOpen.remove(evicted);
    }
  }

  String _ensureDraft(
    Map<String, SessionWorkspace> workspaces,
    String workingDirectory,
  ) {
    final draftKey = _nextDraftKey();
    workspaces[draftKey] = _draftWorkspace(workingDirectory);
    return draftKey;
  }

  bool _isEmptyDraft(SessionWorkspace workspace) =>
      workspace.sessionId == null &&
      workspace.messages.isEmpty &&
      !workspace.busy;

  SessionWorkspace _draftWorkspace(
    String workingDirectory, {
    ModelDescriptor? model,
    String? reasoningEffort,
  }) {
    final selected = model ?? _defaultModel();
    return SessionWorkspace(
      workingDirectory: workingDirectory,
      activeModel: selected,
      reasoningEffort:
          reasoningEffort ?? selected.reasoningEfforts.firstOrNull?.value,
    );
  }

  ({ModelDescriptor model, String? effort}) _selectionFromTurns(
    List<Turn> turns,
  ) {
    for (final turn in turns.reversed) {
      final model = turn.model;
      if (model == null) {
        continue;
      }
      final descriptor = _catalogDescriptor(model);
      if (descriptor == null) {
        break;
      }
      final effort = turn.reasoningEffort;
      final validEffort =
          effort != null &&
              descriptor.reasoningEfforts.any(
                (option) => option.value == effort,
              )
          ? effort
          : descriptor.reasoningEfforts.firstOrNull?.value;
      return (model: descriptor, effort: validEffort);
    }
    final model = _defaultModel();
    return (model: model, effort: model.reasoningEfforts.firstOrNull?.value);
  }

  ModelDescriptor _defaultModel() =>
      _descriptorFor(_environment.runtime.defaultModel);

  ModelDescriptor? _catalogDescriptor(ModelRef ref) {
    for (final model in _environment.models) {
      if (model.ref == ref) {
        return model;
      }
    }
    return null;
  }

  void _cancelAll() {
    for (final token in _cancellations.values) {
      token.cancel();
    }
    _cancellations.clear();
    _permissionSub?.cancel();
    _permissionSub = null;
  }

  String _nextDraftKey() => 'draft-${_draftSerial++}';

  String _nextId() => 'local-${_localId++}';

  /// The plan marker for [status]: a check for completed steps, a dot for
  /// pending ones, and a bullet for in-progress steps.
  static String _planMarker(String status) => switch (status) {
    'completed' => '✓',
    'in_progress' => '●',
    _ => '○',
  };

  ModelDescriptor _descriptorFor(ModelRef ref) =>
      _catalogDescriptor(ref) ?? ModelDescriptor(ref: ref);

  List<String> _selectedSkills(String text) {
    final sessionId =
        state.workspaces[state.activeKey]?.sessionId ?? state.active.sessionId;
    if (sessionId == null) return const [];
    final available = {
      for (final command in _environment.runtime.commandsFor(sessionId))
        command.name,
    };
    final selected = <String>[];
    for (final token in text.split(RegExp(r'\s+'))) {
      if (!token.startsWith('/') || token.length == 1) {
        continue;
      }
      final name = token.substring(1);
      if (available.contains(name) && !selected.contains(name)) {
        selected.add(name);
      }
    }
    return selected;
  }

  /// Whether any timeline message carries image content.
  static bool _timelineHasImages(List<TimelineItem> timeline) {
    for (final item in timeline) {
      final content = switch (item) {
        UserMessageItem(:final content) => content,
        AssistantMessageItem(:final content) => content,
        _ => null,
      };
      if (content != null && content.any((part) => part is ImageContent)) {
        return true;
      }
    }
    return false;
  }

  List<WorkspaceMessage> _messagesFromTimeline(List<TimelineItem> timeline) {
    final messages = <WorkspaceMessage>[];
    final calls = <ToolCallId, int>{};
    for (final item in timeline) {
      switch (item) {
        case UserMessageItem(:final content):
          final text = textFromContent(content);
          final imageSources = [
            for (final part in content)
              if (part is ImageContent) part.source,
          ];
          if (text.isNotEmpty || imageSources.isNotEmpty) {
            messages.add(
              WorkspaceMessage(
                id: item.id.value,
                kind: WorkspaceMessageKind.user,
                text: text,
                imageSources: imageSources,
              ),
            );
          }
        case AssistantMessageItem(:final content, :final reasoning):
          if (reasoning.isNotEmpty) {
            messages.add(
              WorkspaceMessage(
                id: _nextId(),
                kind: WorkspaceMessageKind.reasoning,
                text: reasoning,
              ),
            );
          }
          final text = textFromContent(content);
          if (text.isNotEmpty) {
            messages.add(
              WorkspaceMessage(
                id: item.id.value,
                kind: WorkspaceMessageKind.assistant,
                text: text,
              ),
            );
          }
        case ToolCallItem(:final call):
          calls[call.id] = messages.length;
          messages.add(
            WorkspaceMessage(
              id: item.id.value,
              kind: WorkspaceMessageKind.tool,
              text: '',
              toolName: call.name,
              arguments: call.arguments,
              isRunning: true,
            ),
          );
        case ToolResultItem(:final callId, :final content, :final isError):
          final index = calls[callId];
          if (index != null) {
            messages[index] = messages[index].copyWith(
              text: content,
              isError: isError,
              isRunning: false,
            );
          }
      }
    }
    return messages;
  }
}

/// Exposes the workspace state and its controller to presentation code.
final workspaceProvider =
    NotifierProvider.autoDispose<WorkspaceController, WorkspaceState>(
      WorkspaceController.new,
    );

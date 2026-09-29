import 'package:atlas_runtime/atlas_runtime.dart';

import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';

/// Activity phase of a turn, mirroring the TUI status line.
enum TurnPhase {
  /// No turn is active.
  idle,

  /// The model is producing output or running tools.
  working,

  /// The model is producing reasoning text.
  thinking,

  /// The runtime is compacting context.
  compacting,
}

/// Cached transcript and turn status for one workspace session or draft.
final class SessionWorkspace({
  /// Working directory used by tools, the file browser, and the terminal.
  required final String workingDirectory,

  /// Model used by subsequent turns on this session.
  required final ModelDescriptor activeModel,
  List<WorkspaceMessage> messages = const [],

  /// Agent commands advertised for this session.
  List<AgentCommand> commands = const [],

  /// Persisted session id, or null for a draft that has not started a turn.
  final SessionId? sessionId,

  /// Whether a turn or compaction is active on this session.
  final bool busy = false,

  /// Activity phase of the active turn or compaction.
  final TurnPhase turnPhase = TurnPhase.idle,

  /// When the active turn or compaction started, used for elapsed time.
  final DateTime? turnStartedAt,

  /// Whether this session finished a turn in the current app session.
  final bool hasCompletedTurn = false,

  /// Token usage reported by the most recently completed turn.
  final int contextTokens = 0,

  /// Whether the loaded conversation contains image content.
  final bool hasImages = false,

  /// Whether the workspace tools sidebar shows the terminal for this session.
  final bool showTerminal = false,

  /// Provider-local reasoning effort for subsequent turns on this session.
  final String? reasoningEffort,

  /// Agent session mode for subsequent turns, when the agent offers modes.
  final String? mode,
}) {
  /// Creates a session workspace cache.
  this
    : messages = List.unmodifiable(messages),
      commands = List.unmodifiable(commands);

  /// Conversation items in occurrence order.
  final List<WorkspaceMessage> messages;

  /// Agent command catalog rendered by the composer.
  final List<AgentCommand> commands;

  /// Returns a copy with the given fields replaced.
  SessionWorkspace copyWith({
    SessionId? sessionId,
    String? workingDirectory,
    List<WorkspaceMessage>? messages,
    List<AgentCommand>? commands,
    bool? busy,
    TurnPhase? turnPhase,
    Object? turnStartedAt = _unset,
    bool? hasCompletedTurn,
    int? contextTokens,
    bool? hasImages,
    bool? showTerminal,
    ModelDescriptor? activeModel,
    Object? reasoningEffort = _unset,
    Object? mode = _unset,
  }) => SessionWorkspace(
    sessionId: sessionId ?? this.sessionId,
    workingDirectory: workingDirectory ?? this.workingDirectory,
    messages: messages ?? this.messages,
    commands: commands ?? this.commands,
    busy: busy ?? this.busy,
    turnPhase: turnPhase ?? this.turnPhase,
    turnStartedAt: identical(turnStartedAt, _unset)
        ? this.turnStartedAt
        : turnStartedAt as DateTime?,
    hasCompletedTurn: hasCompletedTurn ?? this.hasCompletedTurn,
    contextTokens: contextTokens ?? this.contextTokens,
    hasImages: hasImages ?? this.hasImages,
    showTerminal: showTerminal ?? this.showTerminal,
    activeModel: activeModel ?? this.activeModel,
    reasoningEffort: identical(reasoningEffort, _unset)
        ? this.reasoningEffort
        : reasoningEffort as String?,
    mode: identical(mode, _unset) ? this.mode : mode as String?,
  );

  static const _unset = Object();
}

/// Immutable state of one Flutter workspace, exposed by [WorkspaceController].
///
/// Copies share unchanged collections with the state they were copied from, so
/// a listener that watches one collection (the session list, pending
/// permission requests, the cache keys) is not notified while an unrelated
/// session streams a turn.
final class WorkspaceState {
  /// Creates a workspace state that copies its collections defensively.
  WorkspaceState({
    /// Cache key of the focused session or draft.
    required this.activeKey,
    required Map<String, SessionWorkspace> workspaces,
    required List<SessionSummary> sessions,

    /// Whether the session sidebar is refreshing.
    this.loadingSessions = false,
    List<PermissionRequest> pendingPermissions = const [],
    List<ModelDescriptor> models = const [],
    List<ModeOption> modes = const [],
  }) : workspaces = Map<String, SessionWorkspace>.unmodifiable(workspaces),
       sessions = List.unmodifiable(sessions),
       pendingPermissions = List.unmodifiable(pendingPermissions),
       models = List.unmodifiable(models),
       modes = List.unmodifiable(modes),
       workspaceKeys = List.unmodifiable(workspaces.keys),
       runningSessionIds = _runningSessionIds(workspaces),
       completedSessionIds = _completedSessionIds(workspaces);

  /// Creates a state that reuses the immutable collections of a previous one.
  ///
  /// [copyWith] uses this path so that unchanged collections keep their
  /// instance, which is what `select` watchers compare.
  WorkspaceState._shared({
    required this.activeKey,
    required this.workspaces,
    required this.workspaceKeys,
    required this.sessions,
    required this.loadingSessions,
    required this.pendingPermissions,
    required this.models,
    required this.modes,
    required this.runningSessionIds,
    required this.completedSessionIds,
  });

  /// Cache key of the focused session or draft.
  final String activeKey;

  /// Per-session transcripts and turn status, including background runs.
  final Map<String, SessionWorkspace> workspaces;

  /// Cache keys of [workspaces], in insertion order.
  final List<String> workspaceKeys;

  /// Sessions for the sidebar, newest first.
  final List<SessionSummary> sessions;

  /// Whether the session sidebar is refreshing.
  final bool loadingSessions;

  /// Agent permission requests awaiting a user decision, in arrival order.
  final List<PermissionRequest> pendingPermissions;

  /// Available models exposed without runtime access from views.
  final List<ModelDescriptor> models;

  /// Available session modes exposed without runtime access from views.
  final List<ModeOption> modes;

  /// Persisted sessions that currently have a turn or compaction in flight.
  final Set<SessionId> runningSessionIds;

  /// Persisted sessions that finished a turn in this app session.
  final Set<SessionId> completedSessionIds;

  /// Focused session cache.
  SessionWorkspace get active {
    final workspace = workspaces[activeKey];
    if (workspace == null) {
      throw StateError('missing workspace cache for $activeKey');
    }
    return workspace;
  }

  /// Conversation items of the focused session.
  List<WorkspaceMessage> get messages => active.messages;

  /// Whether the focused session has a turn in flight.
  bool get busy => active.busy;

  /// Persisted id of the focused session, or null for a draft.
  SessionId? get sessionId => active.sessionId;

  /// Working directory of the focused session.
  String get workingDirectory => active.workingDirectory;

  /// Token usage of the focused session.
  int get contextTokens => active.contextTokens;

  /// Whether the focused conversation contains image content.
  bool get hasImages => active.hasImages;

  /// Whether the focused session's tools sidebar shows the terminal.
  bool get showTerminal => active.showTerminal;

  /// Model of the focused session.
  ModelDescriptor get activeModel => active.activeModel;

  /// Reasoning effort of the focused session.
  String? get reasoningEffort => active.reasoningEffort;

  /// Agent session mode of the focused session.
  String? get mode => active.mode;

  /// Returns a copy with the given fields replaced.
  ///
  /// Collections that did not change keep their instance, so listeners that
  /// watch one collection are not rebuilt for every streaming token.
  WorkspaceState copyWith({
    String? activeKey,
    Map<String, SessionWorkspace>? workspaces,
    List<SessionSummary>? sessions,
    bool? loadingSessions,
    List<PermissionRequest>? pendingPermissions,
    List<ModelDescriptor>? models,
    List<ModeOption>? modes,
  }) {
    final nextWorkspaces = workspaces ?? this.workspaces;
    final running = _runningSessionIds(nextWorkspaces);
    final completed = _completedSessionIds(nextWorkspaces);
    return WorkspaceState._shared(
      activeKey: activeKey ?? this.activeKey,
      workspaces: workspaces == null
          ? nextWorkspaces
          : Map<String, SessionWorkspace>.unmodifiable(nextWorkspaces),
      workspaceKeys: _sameKeyOrder(nextWorkspaces, workspaceKeys)
          ? workspaceKeys
          : List.unmodifiable(nextWorkspaces.keys),
      sessions: sessions == null ? this.sessions : List.unmodifiable(sessions),
      loadingSessions: loadingSessions ?? this.loadingSessions,
      pendingPermissions: pendingPermissions == null
          ? this.pendingPermissions
          : List.unmodifiable(pendingPermissions),
      models: models == null ? this.models : List.unmodifiable(models),
      modes: modes == null ? this.modes : List.unmodifiable(modes),
      runningSessionIds: _sameIds(running, runningSessionIds)
          ? runningSessionIds
          : running,
      completedSessionIds: _sameIds(completed, completedSessionIds)
          ? completedSessionIds
          : completed,
    );
  }

  /// Whether [workspaces] holds exactly the keys of [keys], in the same order.
  static bool _sameKeyOrder(
    Map<String, SessionWorkspace> workspaces,
    List<String> keys,
  ) {
    if (workspaces.length != keys.length) {
      return false;
    }
    var index = 0;
    for (final key in workspaces.keys) {
      if (keys[index++] != key) {
        return false;
      }
    }
    return true;
  }

  /// Whether both sets hold the same session ids.
  static bool _sameIds(Set<SessionId> left, Set<SessionId> right) {
    if (left.length != right.length) {
      return false;
    }
    for (final id in left) {
      if (!right.contains(id)) {
        return false;
      }
    }
    return true;
  }

  /// Persisted ids of sessions with a turn or compaction in flight.
  static Set<SessionId> _runningSessionIds(
    Map<String, SessionWorkspace> workspaces,
  ) => {
    for (final workspace in workspaces.values)
      if (workspace.busy && workspace.sessionId != null) workspace.sessionId!,
  };

  /// Persisted ids of sessions that finished a turn in this app session.
  static Set<SessionId> _completedSessionIds(
    Map<String, SessionWorkspace> workspaces,
  ) => {
    for (final workspace in workspaces.values)
      if (workspace.hasCompletedTurn &&
          !workspace.busy &&
          workspace.sessionId != null)
        workspace.sessionId!,
  };
}

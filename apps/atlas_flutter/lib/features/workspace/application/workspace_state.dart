import 'package:atlas_runtime/atlas_runtime.dart';

import 'workspace_message.dart';

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
final class SessionWorkspace {
  /// Creates a session workspace cache.
  SessionWorkspace({
    required this.workingDirectory,
    required this.activeModel,
    List<WorkspaceMessage> messages = const [],
    this.sessionId,
    this.busy = false,
    this.turnPhase = TurnPhase.idle,
    this.turnStartedAt,
    this.hasCompletedTurn = false,
    this.contextTokens = 0,
    this.hasImages = false,
    this.showTerminal = false,
    this.reasoningEffort,
    this.mode,
  }) : messages = List.unmodifiable(messages);

  /// Persisted session id, or null for a draft that has not started a turn.
  final SessionId? sessionId;

  /// Working directory used by tools, the file browser, and the terminal.
  final String workingDirectory;

  /// Conversation items in occurrence order.
  final List<WorkspaceMessage> messages;

  /// Whether a turn or compaction is active on this session.
  final bool busy;

  /// Activity phase of the active turn or compaction.
  final TurnPhase turnPhase;

  /// When the active turn or compaction started, used for elapsed time.
  final DateTime? turnStartedAt;

  /// Whether this session finished a turn in the current app session.
  final bool hasCompletedTurn;

  /// Token usage reported by the most recently completed turn.
  final int contextTokens;

  /// Whether the loaded conversation contains image content.
  final bool hasImages;

  /// Whether the workspace tools sidebar shows the terminal for this session.
  final bool showTerminal;

  /// Model used by subsequent turns on this session.
  final ModelDescriptor activeModel;

  /// Provider-local reasoning effort for subsequent turns on this session.
  final String? reasoningEffort;

  /// Agent session mode for subsequent turns, when the agent offers modes.
  final String? mode;

  /// Returns a copy with the given fields replaced.
  SessionWorkspace copyWith({
    SessionId? sessionId,
    String? workingDirectory,
    List<WorkspaceMessage>? messages,
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
final class WorkspaceState {
  /// Creates a workspace state.
  WorkspaceState({
    required this.activeKey,
    required Map<String, SessionWorkspace> workspaces,
    required List<SessionSummary> sessions,
    this.loadingSessions = false,
    List<PermissionRequest> pendingPermissions = const [],
  }) : workspaces = Map<String, SessionWorkspace>.unmodifiable(workspaces),
       sessions = List.unmodifiable(sessions),
       pendingPermissions = List.unmodifiable(pendingPermissions);

  /// Cache key of the focused session or draft.
  final String activeKey;

  /// Per-session transcripts and turn status, including background runs.
  final Map<String, SessionWorkspace> workspaces;

  /// Sessions for the sidebar, newest first.
  final List<SessionSummary> sessions;

  /// Whether the session sidebar is refreshing.
  final bool loadingSessions;

  /// Agent permission requests awaiting a user decision, in arrival order.
  final List<PermissionRequest> pendingPermissions;

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

  /// Persisted sessions that currently have a turn or compaction in flight.
  Set<SessionId> get runningSessionIds => {
    for (final workspace in workspaces.values)
      if (workspace.busy && workspace.sessionId != null) workspace.sessionId!,
  };

  /// Persisted sessions that finished a turn in this app session.
  Set<SessionId> get completedSessionIds => {
    for (final workspace in workspaces.values)
      if (workspace.hasCompletedTurn &&
          !workspace.busy &&
          workspace.sessionId != null)
        workspace.sessionId!,
  };

  /// Display title for the central workspace.
  String get sessionTitle {
    final activeId = sessionId;
    if (activeId == null) {
      return 'New session';
    }
    for (final session in sessions) {
      if (session.id == activeId) {
        return session.title.isEmpty ? 'Untitled session' : session.title;
      }
    }
    return 'Session';
  }

  /// Returns a copy with the given fields replaced.
  WorkspaceState copyWith({
    String? activeKey,
    Map<String, SessionWorkspace>? workspaces,
    List<SessionSummary>? sessions,
    bool? loadingSessions,
    List<PermissionRequest>? pendingPermissions,
  }) => WorkspaceState(
    activeKey: activeKey ?? this.activeKey,
    workspaces: workspaces ?? this.workspaces,
    sessions: sessions ?? this.sessions,
    loadingSessions: loadingSessions ?? this.loadingSessions,
    pendingPermissions: pendingPermissions ?? this.pendingPermissions,
  );
}

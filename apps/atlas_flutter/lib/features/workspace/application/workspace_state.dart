import 'package:atlas_runtime/atlas_runtime.dart';

import 'workspace_message.dart';

/// Cached transcript and turn status for one workspace session or draft.
final class SessionWorkspace {
  /// Creates a session workspace cache.
  SessionWorkspace({
    required this.workingDirectory,
    List<WorkspaceMessage> messages = const [],
    this.sessionId,
    this.busy = false,
    this.hasCompletedTurn = false,
    this.contextTokens = 0,
    this.hasImages = false,
    this.draft = '',
  }) : messages = List.unmodifiable(messages);

  /// Persisted session id, or null for a draft that has not started a turn.
  final SessionId? sessionId;

  /// Working directory used by tools, the file browser, and the terminal.
  final String workingDirectory;

  /// Conversation items in occurrence order.
  final List<WorkspaceMessage> messages;

  /// Whether a turn or compaction is active on this session.
  final bool busy;

  /// Whether this session finished a turn in the current app session.
  final bool hasCompletedTurn;

  /// Token usage reported by the most recently completed turn.
  final int contextTokens;

  /// Whether the loaded conversation contains image content.
  final bool hasImages;

  /// Unsent composer text for this session or draft.
  final String draft;

  /// Returns a copy with the given fields replaced.
  SessionWorkspace copyWith({
    SessionId? sessionId,
    String? workingDirectory,
    List<WorkspaceMessage>? messages,
    bool? busy,
    bool? hasCompletedTurn,
    int? contextTokens,
    bool? hasImages,
    String? draft,
  }) => SessionWorkspace(
    sessionId: sessionId ?? this.sessionId,
    workingDirectory: workingDirectory ?? this.workingDirectory,
    messages: messages ?? this.messages,
    busy: busy ?? this.busy,
    hasCompletedTurn: hasCompletedTurn ?? this.hasCompletedTurn,
    contextTokens: contextTokens ?? this.contextTokens,
    hasImages: hasImages ?? this.hasImages,
    draft: draft ?? this.draft,
  );
}

/// Immutable state of one Flutter workspace, exposed by [WorkspaceController].
final class WorkspaceState {
  /// Creates a workspace state.
  WorkspaceState({
    required this.activeKey,
    required Map<String, SessionWorkspace> workspaces,
    required List<SessionSummary> sessions,
    required this.activeModel,
    this.reasoningEffort,
    this.loadingSessions = false,
  }) : workspaces = Map<String, SessionWorkspace>.unmodifiable(workspaces),
       sessions = List.unmodifiable(sessions);

  /// Cache key of the focused session or draft.
  final String activeKey;

  /// Per-session transcripts and turn status, including background runs.
  final Map<String, SessionWorkspace> workspaces;

  /// Sessions for the sidebar, newest first.
  final List<SessionSummary> sessions;

  /// The model used by subsequent turns.
  final ModelDescriptor activeModel;

  /// The provider-local reasoning effort used by subsequent turns.
  final String? reasoningEffort;

  /// Whether the session sidebar is refreshing.
  final bool loadingSessions;

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

  /// Unsent composer text of the focused session.
  String get draft => active.draft;

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

  /// Sentinel distinguishing an unset optional field from an explicit null.
  static const _unset = Object();

  /// Returns a copy with the given fields replaced.
  WorkspaceState copyWith({
    String? activeKey,
    Map<String, SessionWorkspace>? workspaces,
    List<SessionSummary>? sessions,
    ModelDescriptor? activeModel,
    Object? reasoningEffort = _unset,
    bool? loadingSessions,
  }) => WorkspaceState(
    activeKey: activeKey ?? this.activeKey,
    workspaces: workspaces ?? this.workspaces,
    sessions: sessions ?? this.sessions,
    activeModel: activeModel ?? this.activeModel,
    reasoningEffort: identical(reasoningEffort, _unset)
        ? this.reasoningEffort
        : reasoningEffort as String?,
    loadingSessions: loadingSessions ?? this.loadingSessions,
  );
}

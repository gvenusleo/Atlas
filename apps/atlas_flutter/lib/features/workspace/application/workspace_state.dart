import 'package:atlas_runtime/atlas_runtime.dart';

import 'workspace_message.dart';

/// Immutable state of one Flutter workspace, exposed by [WorkspaceController].
final class WorkspaceState {
  /// Creates a workspace state.
  WorkspaceState({
    required List<WorkspaceMessage> messages,
    required List<SessionSummary> sessions,
    required this.activeModel,
    required this.workingDirectory,
    this.reasoningEffort,
    this.sessionId,
    this.busy = false,
    this.loadingSessions = false,
    this.contextTokens = 0,
    this.hasImages = false,
  }) : messages = List.unmodifiable(messages),
       sessions = List.unmodifiable(sessions);

  /// Conversation items in occurrence order.
  final List<WorkspaceMessage> messages;

  /// Sessions for the active working directory.
  final List<SessionSummary> sessions;

  /// The model used by subsequent turns.
  final ModelDescriptor activeModel;

  /// The provider-local reasoning effort used by subsequent turns.
  final String? reasoningEffort;

  /// The currently loaded session, or null for a new session.
  final SessionId? sessionId;

  /// The directory used by the runtime, file browser, and terminal.
  final String workingDirectory;

  /// Whether a turn or compaction operation is active.
  final bool busy;

  /// Whether the session sidebar is refreshing.
  final bool loadingSessions;

  /// Token usage reported by the most recently completed turn.
  final int contextTokens;

  /// Whether the loaded conversation contains image content.
  final bool hasImages;

  /// Display title for the central workspace.
  String get sessionTitle {
    final active = sessionId;
    if (active == null) {
      return 'New session';
    }
    for (final session in sessions) {
      if (session.id == active) {
        return session.title.isEmpty ? 'Untitled session' : session.title;
      }
    }
    return 'Session';
  }

  /// Sentinel distinguishing an unset optional field from an explicit null.
  static const _unset = Object();

  /// Returns a copy with the given fields replaced.
  WorkspaceState copyWith({
    List<WorkspaceMessage>? messages,
    List<SessionSummary>? sessions,
    ModelDescriptor? activeModel,
    Object? reasoningEffort = _unset,
    Object? sessionId = _unset,
    String? workingDirectory,
    bool? busy,
    bool? loadingSessions,
    int? contextTokens,
    bool? hasImages,
  }) => WorkspaceState(
    messages: messages ?? this.messages,
    sessions: sessions ?? this.sessions,
    activeModel: activeModel ?? this.activeModel,
    reasoningEffort: identical(reasoningEffort, _unset)
        ? this.reasoningEffort
        : reasoningEffort as String?,
    sessionId: identical(sessionId, _unset)
        ? this.sessionId
        : sessionId as SessionId?,
    workingDirectory: workingDirectory ?? this.workingDirectory,
    busy: busy ?? this.busy,
    loadingSessions: loadingSessions ?? this.loadingSessions,
    contextTokens: contextTokens ?? this.contextTokens,
    hasImages: hasImages ?? this.hasImages,
  );
}

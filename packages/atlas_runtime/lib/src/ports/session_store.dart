import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/timeline.dart';
import '../domain/turn.dart';

/// Thrown when a requested session does not exist.
final class const SessionNotFoundException(
  /// The missing identifier.
  final SessionId sessionId,
) implements Exception {
  /// Creates a missing-session error.
  this;

  @override
  String toString() => 'Session not found: $sessionId';
}

/// Options for listing sessions.
final class const SessionQuery({
  /// Restricts results to a working directory.
  final String? workingDirectory,

  /// The opaque pagination cursor.
  final String? cursor,

  /// The requested page size.
  final int limit = 20,
}) {
  /// Creates a session query.
  this;
}

/// The initial metadata and user item for a new turn.
final class const BeginTurn({
  /// The session to create or update.
  required final Session session,

  /// The running turn to persist.
  required final Turn turn,

  /// The first timeline item for the turn.
  required final UserMessageItem userMessage,
}) {
  /// Creates a begin-turn operation.
  this;
}

/// A model step and its generated continuation state.
final class const PersistedModelStep({
  /// The completed assistant item.
  required final AssistantMessageItem assistantMessage,

  /// Tool calls requested by this assistant item.
  required final List<ToolCallItem> toolCalls,

  /// Provider continuation state, if returned.
  final ModelCheckpoint? checkpoint,
}) {
  /// Creates a model-step persistence operation.
  this;
}

/// Persists session state and ordered turn history.
abstract interface class SessionStore {
  /// Creates a new session.
  Future<void> createSession(Session session);

  /// Loads session state and timeline items after its compaction boundary.
  Future<SessionSnapshot> loadSession(SessionId sessionId);

  /// Lists session summaries in descending update order.
  Future<SessionPage> listSessions(SessionQuery query);

  /// Persists a running turn and its user message atomically.
  Future<void> beginTurn(BeginTurn operation);

  /// Appends a completed model step atomically.
  Future<void> appendModelStep(
    SessionId sessionId,
    PersistedModelStep operation,
  );

  /// Appends a tool result after execution.
  Future<void> appendToolResult(SessionId sessionId, ToolResultItem item);

  /// Marks a turn as completed, failed, or cancelled.
  Future<void> finishTurn(SessionId sessionId, Turn turn);

  /// Saves a checkpoint at a persisted timeline boundary. The boundary may
  /// split a turn but must not split a tool call/result pair.
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  );

  /// Deletes a session and all of its dependent records.
  Future<void> deleteSession(SessionId sessionId);

  /// Renames a session's display title.
  Future<void> renameSession(SessionId sessionId, String title);
}

/// Optional persistence capability for lightweight metadata reads.
abstract interface class SessionMetadataStore {
  /// Loads only durable session metadata without timeline rows.
  Future<Session> loadSessionMetadata(SessionId sessionId);
}

/// Optional persistence capability for session-level model settings.
abstract interface class SessionConfigStore {
  /// Persists the selected model and reasoning effort for a session.
  Future<void> updateSessionConfig(
    SessionId sessionId,
    ModelRef? model,
    String? reasoningEffort,
  );
}

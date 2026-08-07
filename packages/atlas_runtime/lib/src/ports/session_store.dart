import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/timeline.dart';
import '../domain/turn.dart';

/// Thrown when a requested session does not exist.
final class SessionNotFoundException implements Exception {
  /// Creates a missing-session error.
  const SessionNotFoundException(this.sessionId);

  /// The missing identifier.
  final SessionId sessionId;

  @override
  String toString() => 'Session not found: $sessionId';
}

/// Options for listing sessions.
final class SessionQuery {
  /// Creates a session query.
  const SessionQuery({this.workingDirectory, this.cursor, this.limit = 20});

  /// Restricts results to a working directory.
  final String? workingDirectory;

  /// The opaque pagination cursor.
  final String? cursor;

  /// The requested page size.
  final int limit;
}

/// The initial metadata and user item for a new turn.
final class BeginTurn {
  /// Creates a begin-turn operation.
  const BeginTurn({
    required this.session,
    required this.turn,
    required this.userMessage,
  });

  /// The session to create or update.
  final Session session;

  /// The running turn to persist.
  final Turn turn;

  /// The first timeline item for the turn.
  final UserMessageItem userMessage;
}

/// A model step and its generated continuation state.
final class PersistedModelStep {
  /// Creates a model-step persistence operation.
  const PersistedModelStep({
    required this.assistantMessage,
    required this.toolCalls,
    this.checkpoint,
  });

  /// The completed assistant item.
  final AssistantMessageItem assistantMessage;

  /// Tool calls requested by this assistant item.
  final List<ToolCallItem> toolCalls;

  /// Provider continuation state, if returned.
  final ModelCheckpoint? checkpoint;
}

/// Persists session state and ordered turn history.
abstract interface class SessionStore {
  /// Creates a new session.
  Future<void> createSession(Session session);

  /// Loads a session with all durable turns and timeline items.
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

  /// Saves the latest compaction checkpoint.
  Future<void> saveCompaction(
    SessionId sessionId,
    CompactionCheckpoint checkpoint,
  );

  /// Deletes a session and all of its dependent records.
  Future<void> deleteSession(SessionId sessionId);
}

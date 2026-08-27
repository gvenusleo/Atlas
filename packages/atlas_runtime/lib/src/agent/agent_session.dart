import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/turn.dart';
import '../ports/cancellation.dart';
import '../skills/skill.dart';

/// Core session contract shared by local and ACP-backed clients.
abstract interface class AgentSession {
  /// The model used when a turn does not provide an override.
  ModelRef get defaultModel;

  /// Executes one turn and emits events in occurrence order.
  Stream<AgentEvent> run(TurnRequest request);

  /// Manually compacts a session.
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    CancellationToken? cancellation,
  });

  /// Lists persisted sessions.
  Future<SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  });

  /// Creates and persists a blank session.
  Future<Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const <String>[],
  });

  /// Loads a session and its timeline.
  Future<SessionSnapshot> loadSession(SessionId sessionId);

  /// Deletes a session.
  Future<void> deleteSession(SessionId sessionId);

  /// Renames a session.
  Future<void> renameSession(SessionId sessionId, String title);

  /// Returns the default model context window.
  Future<int> contextWindowSize();
}

/// Optional presentation capabilities exposed by protocol-backed sessions.
abstract interface class PresentationAgentSession implements AgentSession {
  /// Returns a display title when available.
  String? titleFor(SessionId sessionId);

  /// Returns advertised slash commands.
  List<AgentCommand> commandsFor(SessionId sessionId);

  /// Returns advertised operating modes.
  List<ModeOption> get modeOptions;

  /// Returns the current mode.
  String? modeFor(SessionId sessionId);

  /// Changes the current mode.
  Future<void> setMode(SessionId sessionId, String modeId);
}

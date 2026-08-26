import '../domain/events.dart';
import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/session.dart';
import '../domain/turn.dart';
import '../ports/cancellation.dart';
import '../skills/skill.dart';

/// The runtime surface consumed by presentation and protocol adapters.
///
/// [AgentRuntime] is the local implementation; remote and protocol clients
/// (such as an ACP client) implement the same surface so presentation code
/// never depends on a concrete runtime.
abstract interface class RuntimeService {
  /// The model used when a turn does not provide an override.
  ModelRef get defaultModel;

  /// Executes one turn and emits events in their exact occurrence order.
  Stream<AgentEvent> run(TurnRequest request);

  /// Manually compacts [sessionId] without the threshold check.
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    CancellationToken? cancellation,
  });

  /// Lists session summaries in descending update order.
  Future<SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  });

  /// Creates a new blank session and persists it.
  Future<Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const <String>[],
  });

  /// Loads one session and its timeline for display or resume.
  Future<SessionSnapshot> loadSession(SessionId sessionId);

  /// Deletes [sessionId] and all of its dependent records.
  Future<void> deleteSession(SessionId sessionId);

  /// Renames [sessionId]'s display title.
  Future<void> renameSession(SessionId sessionId, String title);

  /// The context window size of the default model, or 0 when unknown.
  Future<int> contextWindowSize();

  /// The display title reported for [sessionId], if the runtime has one.
  String? titleFor(SessionId sessionId);

  /// Slash commands advertised for [sessionId].
  List<AgentCommand> commandsFor(SessionId sessionId);

  /// The operating modes offered by the agent, if any.
  List<ModeOption> get modeOptions;

  /// The current mode of [sessionId], if known.
  String? modeFor(SessionId sessionId);

  /// Switches [sessionId] to [modeId].
  Future<void> setMode(SessionId sessionId, String modeId);
}

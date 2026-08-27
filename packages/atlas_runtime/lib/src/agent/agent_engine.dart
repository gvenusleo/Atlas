import '../domain/session_context.dart';
import '../domain/ids.dart';
import 'agent_session.dart';

/// Engine contract used by protocol adapters to execute local agent turns.
///
/// This is intentionally limited to engine concerns; presentation-specific
/// presentation methods remain on [PresentationAgentSession].
abstract interface class AgentEngine implements AgentSession {
  /// Builds the cached session context for a working directory.
  SessionContext sessionContext(String workingDirectory);

  /// Persists protocol-selected model settings for a session.
  Future<void> updateSessionConfig(
    SessionId sessionId,
    ModelRef model,
    String? reasoningEffort,
  );
}

import '../domain/session_context.dart';
import 'runtime_service.dart';

/// Engine contract used by protocol adapters to execute local agent turns.
///
/// This is intentionally limited to engine concerns; presentation-specific
/// capability methods remain on [RuntimeService] until the session contract
/// migration is complete.
abstract interface class AgentEngine implements RuntimeService {
  /// Builds the cached session context for a working directory.
  SessionContext sessionContext(String workingDirectory);
}

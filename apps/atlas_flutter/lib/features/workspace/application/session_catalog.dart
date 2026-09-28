import 'package:atlas_runtime/atlas_runtime.dart';

/// Loads the session list and overlays titles reported by the active agent.
class const SessionCatalog(PresentationAgentSession runtime) {
  final PresentationAgentSession _runtime = runtime;

  /// Loads up to 500 sessions, in runtime order, across directories.
  Future<List<SessionSummary>> load() async {
    final sessions = <SessionSummary>[];
    String? cursor;
    do {
      final page = await _runtime.listSessions(cursor: cursor, limit: 100);
      sessions.addAll(page.items);
      cursor = page.nextCursor;
    } while (cursor != null && sessions.length < 500);
    return [
      for (final session in sessions)
        if (_runtime.titleFor(session.id) case final title?
            when title.isNotEmpty)
          SessionSummary(
            id: session.id,
            title: title,
            workingDirectory: session.workingDirectory,
            updatedAt: session.updatedAt,
          )
        else
          session,
    ];
  }
}

import 'ids.dart';
import 'model.dart';
import 'timeline.dart';
import 'turn.dart';
import 'usage.dart';

/// Metadata and durable settings for a local session.
final class Session {
  /// Creates a session.
  const Session({
    required this.id,
    required this.workingDirectory,
    required this.createdAt,
    required this.updatedAt,
    this.title = '',
    this.additionalDirectories = const <String>[],
    this.compaction,
    this.lastUsage = const TokenUsage(),
  });

  /// The durable session identifier.
  final SessionId id;

  /// The display title derived from the first user message unless renamed.
  final String title;

  /// The primary working directory for tools.
  final String workingDirectory;

  /// Additional working directory roots granted to tools.
  final List<String> additionalDirectories;

  /// Session creation time in UTC.
  final DateTime createdAt;

  /// Last timeline update time in UTC.
  final DateTime updatedAt;

  /// The latest context checkpoint.
  final CompactionCheckpoint? compaction;

  /// Usage reported by the latest completed model response.
  final TokenUsage lastUsage;
}

/// A compact session row used by list views.
final class SessionSummary {
  /// Creates a session summary.
  const SessionSummary({
    required this.id,
    required this.title,
    required this.workingDirectory,
    required this.updatedAt,
    this.lastUsage = const TokenUsage(),
  });

  /// The session identifier.
  final SessionId id;

  /// The display title.
  final String title;

  /// The primary working directory.
  final String workingDirectory;

  /// The last update time in UTC.
  final DateTime updatedAt;

  /// The latest model usage.
  final TokenUsage lastUsage;
}

/// Session state required to continue agent execution.
final class SessionSnapshot {
  /// Creates a session snapshot.
  const SessionSnapshot({
    required this.session,
    required this.turns,
    required this.timeline,
    this.modelCheckpoints = const <ModelCheckpoint>[],
  });

  /// Session metadata.
  final Session session;

  /// Turns ordered by start time.
  final List<Turn> turns;

  /// Active timeline items after the current compaction boundary.
  final List<TimelineItem> timeline;

  /// Provider continuations linked to active assistant timeline items.
  final List<ModelCheckpoint> modelCheckpoints;
}

/// A cursor-paginated session list.
final class SessionPage {
  /// Creates a session page.
  const SessionPage({required this.items, this.nextCursor});

  /// Sessions in descending update order.
  final List<SessionSummary> items;

  /// The cursor for the next page, if any.
  final String? nextCursor;
}

import 'ids.dart';
import 'conversation.dart';
import 'model.dart';
import 'timeline.dart';
import 'turn.dart';
import 'usage.dart';

/// Metadata and durable settings for a local session.
final class const Session({
  /// The durable session identifier.
  required final SessionId id,

  /// The primary working directory for tools.
  required final String workingDirectory,

  /// Session creation time in UTC.
  required final DateTime createdAt,

  /// Last timeline update time in UTC.
  required final DateTime updatedAt,

  /// The display title derived from the first user message unless renamed.
  final String title = '',

  /// Additional working directory roots granted to tools.
  final List<String> additionalDirectories = const <String>[],

  /// The latest context checkpoint.
  final CompactionCheckpoint? compaction,

  /// Usage reported by the latest completed model response.
  final TokenUsage lastUsage = const TokenUsage(),

  /// Session-level model selection.
  final ModelRef? model,

  /// Session-level reasoning effort selection.
  final String? reasoningEffort,
}) {
  /// Creates a session.
  this;
}

/// A compact session row used by list views.
final class const SessionSummary({
  /// The session identifier.
  required final SessionId id,

  /// The display title.
  required final String title,

  /// The primary working directory.
  required final String workingDirectory,

  /// The last update time in UTC.
  required final DateTime updatedAt,

  /// Additional working directory roots granted to tools.
  final List<String> additionalDirectories = const <String>[],

  /// The latest model usage.
  final TokenUsage lastUsage = const TokenUsage(),

  /// Session-level model selection.
  final ModelRef? model,

  /// Session-level reasoning effort selection.
  final String? reasoningEffort,
}) {
  /// Creates a session summary.
  this;
}

/// Session state required to continue agent execution.
final class const SessionSnapshot({
  /// Session metadata.
  required final Session session,

  /// Turns ordered by start time.
  required final List<Turn> turns,

  /// Active timeline items after the current compaction boundary.
  required final List<TimelineItem> timeline,

  /// Provider continuations linked to active assistant timeline items.
  final List<ModelCheckpoint> modelCheckpoints = const <ModelCheckpoint>[],

  /// Presentation-only items reconstructed by remote protocol clients.
  final List<ConversationItem> conversation = const <ConversationItem>[],
}) {
  /// Creates a session snapshot.
  this;
}

/// A cursor-paginated session list.
final class const SessionPage({
  /// Sessions in descending update order.
  required final List<SessionSummary> items,

  /// The cursor for the next page, if any.
  final String? nextCursor,
}) {
  /// Creates a session page.
  this;
}

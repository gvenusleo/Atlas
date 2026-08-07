import 'package:drift/drift.dart';

/// Durable session metadata.
@DataClassName('SessionRow')
class Sessions extends Table {
  /// Serialized session identifier.
  TextColumn get id => text()();

  /// User-facing session title.
  TextColumn get title => text().withDefault(const Constant(''))();

  /// Primary tool working directory.
  TextColumn get workingDirectory => text()();

  /// JSON-encoded additional tool roots.
  TextColumn get additionalDirectoriesJson =>
      text().withDefault(const Constant('[]'))();

  /// Input tokens from the latest model response.
  IntColumn get lastInputTokens => integer().withDefault(const Constant(0))();

  /// Output tokens from the latest model response.
  IntColumn get lastOutputTokens => integer().withDefault(const Constant(0))();

  /// Total tokens from the latest model response.
  IntColumn get lastTotalTokens => integer().withDefault(const Constant(0))();

  /// Cached input tokens read by the latest model response.
  IntColumn get lastCacheReadTokens =>
      integer().withDefault(const Constant(0))();

  /// Cached input tokens written by the latest model response.
  IntColumn get lastCacheWriteTokens =>
      integer().withDefault(const Constant(0))();

  /// UTC creation time.
  DateTimeColumn get createdAt => dateTime()();

  /// UTC last-update time.
  DateTimeColumn get updatedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {id};
}

/// One user turn and its terminal state.
@DataClassName('TurnRow')
class Turns extends Table {
  /// Serialized turn identifier.
  TextColumn get id => text()();

  /// Owning session identifier.
  TextColumn get sessionId =>
      text().references(Sessions, #id, onDelete: KeyAction.cascade)();

  /// Serialized runtime turn status.
  TextColumn get status => text()();

  /// UTC turn start time.
  DateTimeColumn get startedAt => dateTime()();

  /// UTC terminal time.
  DateTimeColumn get completedAt => dateTime().nullable()();

  /// Selected provider identifier.
  TextColumn get providerId => text().nullable()();

  /// Selected model identifier.
  TextColumn get modelId => text().nullable()();

  /// Provider-local reasoning effort.
  TextColumn get reasoningEffort => text().nullable()();

  /// Input token usage.
  IntColumn get inputTokens => integer().withDefault(const Constant(0))();

  /// Output token usage.
  IntColumn get outputTokens => integer().withDefault(const Constant(0))();

  /// Total token usage.
  IntColumn get totalTokens => integer().withDefault(const Constant(0))();

  /// Cached input tokens read.
  IntColumn get cacheReadTokens => integer().withDefault(const Constant(0))();

  /// Cached input tokens written.
  IntColumn get cacheWriteTokens => integer().withDefault(const Constant(0))();

  /// Stable runtime failure code.
  TextColumn get failureCode => text().nullable()();

  /// User-visible failure message.
  TextColumn get failureMessage => text().nullable()();

  /// Cancellation reason.
  TextColumn get cancelReason => text().nullable()();

  @override
  Set<Column<Object>> get primaryKey => {id};
}

/// One strongly typed item in the durable session timeline.
@DataClassName('TimelineItemRow')
class TimelineItems extends Table {
  /// Serialized timeline item identifier.
  TextColumn get id => text()();

  /// Owning session identifier.
  TextColumn get sessionId =>
      text().references(Sessions, #id, onDelete: KeyAction.cascade)();

  /// Owning turn identifier.
  TextColumn get turnId =>
      text().references(Turns, #id, onDelete: KeyAction.cascade)();

  /// Strict session-local order.
  IntColumn get sequence => integer()();

  /// Stable timeline item discriminant.
  TextColumn get kind => text()();

  /// Version of the JSON payload schema.
  IntColumn get payloadVersion => integer().withDefault(const Constant(1))();

  /// Versioned JSON payload.
  TextColumn get payloadJson => text()();

  /// UTC append time.
  DateTimeColumn get occurredAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {id};

  @override
  List<Set<Column<Object>>> get uniqueKeys => [
    {sessionId, sequence},
  ];
}

/// Provider-owned continuation state for an assistant item.
@DataClassName('ModelCheckpointRow')
class ModelCheckpoints extends Table {
  /// Assistant timeline item that owns this continuation.
  TextColumn get timelineItemId =>
      text().references(TimelineItems, #id, onDelete: KeyAction.cascade)();

  /// Provider that can interpret the payload.
  TextColumn get providerId => text()();

  /// Provider-produced reasoning summary.
  TextColumn get reasoningSummary => text().withDefault(const Constant(''))();

  /// Provider-owned continuation payload.
  TextColumn get payloadJson => text().withDefault(const Constant('{}'))();

  /// UTC checkpoint creation time.
  DateTimeColumn get createdAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {timelineItemId};
}

/// The latest context compaction boundary for a session.
@DataClassName('CompactionCheckpointRow')
class CompactionCheckpoints extends Table {
  /// Owning session identifier.
  TextColumn get sessionId =>
      text().references(Sessions, #id, onDelete: KeyAction.cascade)();

  /// Last timeline sequence represented by the summary.
  IntColumn get compactedThroughSequence => integer()();

  /// Compact model context summary.
  TextColumn get summary => text()();

  /// Input token count before compaction.
  IntColumn get inputTokensBefore => integer()();

  /// Input token count after compaction.
  IntColumn get inputTokensAfter => integer()();

  /// UTC checkpoint creation time.
  DateTimeColumn get createdAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {sessionId};
}

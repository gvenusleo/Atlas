import 'package:drift/drift.dart';

/// Durable session metadata and its latest compaction checkpoint.
@DataClassName('SessionRow')
class Sessions extends Table {
  /// Serialized session identifier.
  TextColumn get id => text()();

  /// User-facing session title.
  TextColumn get title => text().withDefault(const Constant(''))();

  /// Primary tool working directory.
  TextColumn get workingDirectory => text()();

  /// Selected provider identifier for the session.
  TextColumn get modelProviderId => text().nullable()();

  /// Selected model identifier for the session.
  TextColumn get modelId => text().nullable()();

  /// Selected reasoning effort for the session.
  TextColumn get reasoningEffort => text().nullable()();

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

  /// Last timeline sequence represented by the compaction summary.
  IntColumn get compactionSequence => integer().nullable()();

  /// Compact model context summary.
  TextColumn get compactionSummary => text().withDefault(const Constant(''))();

  /// Timeline messages after the boundary kept verbatim.
  IntColumn get compactionKeptRecent =>
      integer().withDefault(const Constant(0))();

  /// Input token count before compaction.
  IntColumn get compactionTokensBefore =>
      integer().withDefault(const Constant(0))();

  /// Input token count after compaction.
  IntColumn get compactionTokensAfter =>
      integer().withDefault(const Constant(0))();

  /// UTC checkpoint creation time; null when no checkpoint exists.
  DateTimeColumn get compactionCreatedAt => dateTime().nullable()();

  /// UTC creation time.
  DateTimeColumn get createdAt => dateTime()();

  /// UTC last-update time.
  DateTimeColumn get updatedAt => dateTime()();

  @override
  Set<Column<Object>> get primaryKey => {id};
}

/// One user turn and its terminal state.
@TableIndex(name: 'turns_session_started', columns: {#sessionId, #startedAt})
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

  /// Stable failure category.
  TextColumn get failureKind => text().nullable()();

  /// User-visible failure message.
  TextColumn get failureMessage => text().nullable()();

  /// Bounded provider diagnostic detail.
  TextColumn get providerDetail => text().nullable()();

  /// Cancellation reason.
  TextColumn get cancelReason => text().nullable()();

  @override
  Set<Column<Object>> get primaryKey => {id};
}

/// One strongly typed message in the durable session timeline.
@DataClassName('MessageRow')
class Messages extends Table {
  /// Serialized message identifier.
  TextColumn get id => text()();

  /// Owning session identifier.
  TextColumn get sessionId =>
      text().references(Sessions, #id, onDelete: KeyAction.cascade)();

  /// Owning turn identifier.
  TextColumn get turnId =>
      text().references(Turns, #id, onDelete: KeyAction.cascade)();

  /// Strict session-local order.
  IntColumn get sequence => integer()();

  /// Stable message discriminant.
  TextColumn get kind => text()();

  /// Version of the JSON payload schema.
  IntColumn get payloadVersion => integer().withDefault(const Constant(1))();

  /// Versioned JSON payload; assistant payloads include the provider
  /// continuation when one was returned.
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

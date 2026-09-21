import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart' as runtime;
import 'package:drift/drift.dart';
import 'package:drift/native.dart';

import '../database/database.dart';
import '../mappers/row_mappers.dart';
import '../turn_usage.dart';

/// Drift-backed implementation of the runtime session persistence port.
final class DriftSessionStore._(final AtlasDatabase _database)
    implements
        runtime.SessionStore,
        runtime.SessionConfigStore,
        runtime.SessionMetadataStore {
  /// Creates a store around its owned database.
  this : _mappers = RowMappers();

  /// Opens a persistent store backed by [file].
  factory openFile(File file) =>
      DriftSessionStore._(AtlasDatabase.openFile(file));

  /// Creates an in-memory store for tests and local ephemeral sessions.
  factory inMemory() =>
      DriftSessionStore._(AtlasDatabase(NativeDatabase.memory()));

  final RowMappers _mappers;

  /// Loads the most recent [limit] turns, newest first, for picking a sample.
  ///
  /// The token columns on each sample are the turn's recorded usage, which
  /// only covers the turn's last model response. Each sample also contains all
  /// recorded responses, even before compaction. Both queries share a database
  /// snapshot; failures propagate rather than producing a partial report.
  Future<List<TurnUsageSample>> recentTurnUsage({
    int limit = 200,
  }) => _database.transaction(() async {
    final query =
        _database.select(_database.turns).join([
            innerJoin(
              _database.sessions,
              _database.sessions.id.equalsExp(_database.turns.sessionId),
            ),
          ])
          ..orderBy([
            OrderingTerm.desc(_database.turns.startedAt),
            OrderingTerm.desc(_database.turns.id),
          ])
          ..limit(limit);
    final rows = await query.get();
    final requests = <String, List<runtime.AssistantMessageItem>>{};
    // Bound the IN parameters independently of the user-selected turn limit.
    for (var offset = 0; offset < rows.length; offset += 500) {
      final ids = rows
          .skip(offset)
          .take(500)
          .map((row) => row.readTable(_database.turns).id);
      final messages =
          await (_database.select(_database.messages)
                ..where(
                  (table) =>
                      table.turnId.isIn(ids) &
                      table.kind.equals('assistant_message'),
                )
                ..orderBy([(table) => OrderingTerm.asc(table.sequence)]))
              .get();
      for (final row in messages) {
        final item = _mappers.message(row).item as runtime.AssistantMessageItem;
        requests.putIfAbsent(row.turnId, () => []).add(item);
      }
    }
    return [
      for (final row in rows)
        TurnUsageSample(
          sessionId: row.readTable(_database.turns).sessionId,
          turnId: row.readTable(_database.turns).id,
          title: row.readTable(_database.sessions).title,
          startedAt: row.readTable(_database.turns).startedAt.toUtc(),
          providerId: row.readTable(_database.turns).providerId,
          modelId: row.readTable(_database.turns).modelId,
          inputTokens: row.readTable(_database.turns).inputTokens,
          outputTokens: row.readTable(_database.turns).outputTokens,
          cacheReadTokens: row.readTable(_database.turns).cacheReadTokens,
          cacheWriteTokens: row.readTable(_database.turns).cacheWriteTokens,
          requests: List.unmodifiable(
            requests[row.readTable(_database.turns).id] ?? const [],
          ),
        ),
    ];
  });

  /// Closes the underlying database connection.
  Future<void> close() => _database.close();

  @override
  Future<void> createSession(runtime.Session session) => _database
      .into(_database.sessions)
      .insert(_mappers.sessionCompanion(session));

  @override
  Future<runtime.Session> loadSessionMetadata(
    runtime.SessionId sessionId,
  ) async => _mappers.session(await _sessionRow(sessionId));

  @override
  Future<runtime.SessionSnapshot> loadSession(
    runtime.SessionId sessionId,
  ) async {
    final sessionRow = await _sessionRow(sessionId);
    final turnRows =
        await (_database.select(_database.turns)
              ..where((table) => table.sessionId.equals(sessionId.value))
              ..orderBy([(table) => OrderingTerm.asc(table.startedAt)]))
            .get();
    final session = _mappers.session(sessionRow);
    final compaction = session.compaction;
    final messagesQuery = _database.select(_database.messages)
      ..where((table) => table.sessionId.equals(sessionId.value));
    if (compaction != null && compaction.summary.trim().isNotEmpty) {
      messagesQuery.where(
        (table) => table.sequence.isBiggerThanValue(
          compaction.compactedThroughSequence,
        ),
      );
    }
    messagesQuery.orderBy([(table) => OrderingTerm.asc(table.sequence)]);
    final decoded = (await messagesQuery.get()).map(_mappers.message);
    return runtime.SessionSnapshot(
      session: session,
      turns: List<runtime.Turn>.unmodifiable(turnRows.map(_mappers.turn)),
      timeline: List<runtime.TimelineItem>.unmodifiable(
        decoded.map((value) => value.item),
      ),
      modelCheckpoints: List<runtime.ModelCheckpoint>.unmodifiable(
        decoded.map((value) => value.checkpoint).nonNulls,
      ),
    );
  }

  @override
  Future<runtime.SessionPage> listSessions(runtime.SessionQuery query) async {
    final limit = query.limit <= 0 ? 20 : query.limit.clamp(1, 100);
    final cursor = query.cursor == null ? null : _decodeCursor(query.cursor!);
    final select = _database.select(_database.sessions);
    if (query.workingDirectory != null) {
      select.where(
        (table) => table.workingDirectory.equals(query.workingDirectory!),
      );
    }
    if (cursor != null) {
      select.where(
        (table) =>
            table.updatedAt.isSmallerThanValue(cursor.updatedAt) |
            (table.updatedAt.equals(cursor.updatedAt) &
                table.id.isSmallerThanValue(cursor.id)),
      );
    }
    select
      ..orderBy([
        (table) => OrderingTerm.desc(table.updatedAt),
        (table) => OrderingTerm.desc(table.id),
      ])
      ..limit(limit + 1);
    final rows = await select.get();
    final hasMore = rows.length > limit;
    final pageRows = hasMore ? rows.take(limit).toList() : rows;
    final items = List<runtime.SessionSummary>.unmodifiable(
      pageRows.map(_mappers.sessionSummary),
    );
    return runtime.SessionPage(
      items: items,
      nextCursor: hasMore && pageRows.isNotEmpty
          ? _encodeCursor(pageRows.last.updatedAt, pageRows.last.id)
          : null,
    );
  }

  @override
  Future<void> beginTurn(runtime.BeginTurn operation) => _database.transaction(
    () async {
      _validateBeginTurn(operation);
      await _database
          .into(_database.sessions)
          .insertOnConflictUpdate(_mappers.sessionCompanion(operation.session));
      await _database
          .into(_database.turns)
          .insert(_mappers.turnCompanion(operation.turn));
      await _insertMessage(operation.userMessage);
      await (_database.update(
        _database.sessions,
      )..where((table) => table.id.equals(operation.session.id.value))).write(
        SessionsCompanion(
          title: Value(operation.session.title),
          updatedAt: Value(operation.userMessage.occurredAt.toUtc()),
        ),
      );
    },
  );

  @override
  Future<void> appendModelStep(
    runtime.SessionId sessionId,
    runtime.PersistedModelStep operation,
  ) => _database.transaction(() async {
    await _requireTimelineOwnership(operation.assistantMessage, sessionId);
    final checkpoint = operation.checkpoint;
    if (checkpoint != null &&
        checkpoint.timelineItemId != operation.assistantMessage.id) {
      throw const FormatException(
        'model checkpoint must reference its assistant item',
      );
    }
    await _insertMessage(operation.assistantMessage, checkpoint: checkpoint);
    for (final call in operation.toolCalls) {
      await _requireTimelineOwnership(call, sessionId);
      await _insertMessage(call);
    }
    await _touchSession(
      sessionId,
      operation.assistantMessage.occurredAt,
      operation.assistantMessage.usage,
    );
  });

  @override
  Future<void> appendToolResult(
    runtime.SessionId sessionId,
    runtime.ToolResultItem item,
  ) => _database.transaction(() async {
    await _requireTimelineOwnership(item, sessionId);
    await _insertMessage(item);
    await _touchSession(sessionId, item.occurredAt);
  });

  @override
  Future<void> finishTurn(runtime.SessionId sessionId, runtime.Turn turn) =>
      _database.transaction(() async {
        if (turn.sessionId != sessionId ||
            turn.status == runtime.TurnStatus.running) {
          throw const FormatException(
            'turn must be terminal and match the session',
          );
        }
        final count =
            await (_database.update(_database.turns)..where(
                  (table) =>
                      table.id.equals(turn.id.value) &
                      table.sessionId.equals(sessionId.value),
                ))
                .write(_mappers.turnCompanion(turn));
        if (count == 0) {
          throw runtime.SessionNotFoundException(sessionId);
        }
        await _touchSession(
          sessionId,
          turn.completedAt ?? turn.startedAt,
          turn.usage,
        );
      });

  @override
  Future<void> saveCompaction(
    runtime.SessionId sessionId,
    runtime.CompactionCheckpoint checkpoint,
  ) => _database.transaction(() async {
    if (checkpoint.sessionId != sessionId) {
      throw const FormatException('compaction must match the session');
    }
    await _validateCompactionBoundary(sessionId, checkpoint);
    final count =
        await (_database.update(
          _database.sessions,
        )..where((table) => table.id.equals(sessionId.value))).write(
          SessionsCompanion(
            compactionSequence: Value(checkpoint.compactedThroughSequence),
            compactionSummary: Value(checkpoint.summary),
            compactionKeptRecent: Value(checkpoint.keptRecentMessages),
            compactionTokensBefore: Value(checkpoint.inputTokensBefore),
            compactionTokensAfter: Value(checkpoint.inputTokensAfter),
            compactionCreatedAt: Value(checkpoint.createdAt.toUtc()),
            updatedAt: Value(checkpoint.createdAt.toUtc()),
          ),
        );
    if (count == 0) {
      throw runtime.SessionNotFoundException(sessionId);
    }
  });

  @override
  Future<void> deleteSession(runtime.SessionId sessionId) async {
    final count = await (_database.delete(
      _database.sessions,
    )..where((table) => table.id.equals(sessionId.value))).go();
    if (count == 0) {
      throw runtime.SessionNotFoundException(sessionId);
    }
  }

  @override
  Future<void> renameSession(runtime.SessionId sessionId, String title) async {
    final count =
        await (_database.update(_database.sessions)
              ..where((table) => table.id.equals(sessionId.value)))
            .write(SessionsCompanion(title: Value(title)));
    if (count == 0) {
      throw runtime.SessionNotFoundException(sessionId);
    }
  }

  @override
  Future<void> updateSessionConfig(
    runtime.SessionId sessionId,
    runtime.ModelRef? model,
    String? reasoningEffort,
  ) async {
    await (_database.update(
      _database.sessions,
    )..where((table) => table.id.equals(sessionId.value))).write(
      SessionsCompanion(
        modelProviderId: Value(model?.providerId.value),
        modelId: Value(model?.modelId.value),
        reasoningEffort: Value(reasoningEffort),
      ),
    );
  }

  Future<SessionRow> _sessionRow(runtime.SessionId sessionId) async {
    final row = await (_database.select(
      _database.sessions,
    )..where((table) => table.id.equals(sessionId.value))).getSingleOrNull();
    if (row == null) {
      throw runtime.SessionNotFoundException(sessionId);
    }
    return row;
  }

  Future<void> _insertMessage(
    runtime.TimelineItem item, {
    runtime.ModelCheckpoint? checkpoint,
  }) => _database
      .into(_database.messages)
      .insert(_mappers.messageCompanion(item, checkpoint: checkpoint));

  Future<void> _touchSession(
    runtime.SessionId sessionId,
    DateTime updatedAt, [
    runtime.TokenUsage? usage,
  ]) async {
    final companion = usage == null
        ? SessionsCompanion(updatedAt: Value(updatedAt.toUtc()))
        : SessionsCompanion(
            updatedAt: Value(updatedAt.toUtc()),
            lastInputTokens: Value(usage.inputTokens),
            lastOutputTokens: Value(usage.outputTokens),
            lastTotalTokens: Value(usage.totalTokens),
            lastCacheReadTokens: Value(usage.cacheReadInputTokens),
            lastCacheWriteTokens: Value(usage.cacheWriteInputTokens),
          );
    final count = await (_database.update(
      _database.sessions,
    )..where((table) => table.id.equals(sessionId.value))).write(companion);
    if (count == 0) {
      throw runtime.SessionNotFoundException(sessionId);
    }
  }

  static void _validateBeginTurn(runtime.BeginTurn operation) {
    if (operation.turn.sessionId != operation.session.id ||
        operation.userMessage.sessionId != operation.session.id ||
        operation.userMessage.turnId != operation.turn.id) {
      throw const FormatException(
        'session, turn, and user timeline item must belong together',
      );
    }
  }

  Future<void> _requireTimelineOwnership(
    runtime.TimelineItem item,
    runtime.SessionId sessionId,
  ) async {
    if (item.sessionId != sessionId) {
      throw const FormatException('timeline item must match the session');
    }
    final turn = await (_database.select(
      _database.turns,
    )..where((table) => table.id.equals(item.turnId.value))).getSingleOrNull();
    if (turn == null || turn.sessionId != sessionId.value) {
      throw const FormatException('timeline item turn must match the session');
    }
  }

  Future<void> _validateCompactionBoundary(
    runtime.SessionId sessionId,
    runtime.CompactionCheckpoint checkpoint,
  ) async {
    final boundary =
        await (_database.select(_database.messages)..where(
              (table) =>
                  table.sessionId.equals(sessionId.value) &
                  table.sequence.equals(checkpoint.compactedThroughSequence),
            ))
            .getSingleOrNull();
    if (boundary == null) {
      throw const FormatException(
        'compaction boundary must reference a message',
      );
    }
    final turn = await (_database.select(
      _database.turns,
    )..where((table) => table.id.equals(boundary.turnId))).getSingleOrNull();
    if (turn == null) {
      throw const FormatException(
        'compaction boundary must belong to the session',
      );
    }
    // Active turns may be compacted between model steps. The runtime selects
    // boundaries that preserve complete tool call/result groups.
    final suffix =
        await (_database.select(_database.messages)
              ..where(
                (table) =>
                    table.sessionId.equals(sessionId.value) &
                    table.sequence.isBiggerThanValue(
                      checkpoint.compactedThroughSequence,
                    ),
              )
              ..orderBy([(table) => OrderingTerm.asc(table.sequence)]))
            .get();
    final suffixItems = suffix.map(_mappers.message).map((value) => value.item);
    final callIds = {
      for (final item in suffixItems)
        if (item case runtime.ToolCallItem(:final call)) call.id,
    };
    final resultIds = {
      for (final item in suffixItems)
        if (item case runtime.ToolResultItem(:final callId)) callId,
    };
    if (!callIds.containsAll(resultIds) || !resultIds.containsAll(callIds)) {
      throw const FormatException(
        'compaction boundary must not split a tool call/result pair',
      );
    }
  }

  static String _encodeCursor(DateTime updatedAt, String id) =>
      base64Url.encode(
        utf8.encode('${updatedAt.toUtc().microsecondsSinceEpoch}\u0000$id'),
      );

  static ({DateTime updatedAt, String id}) _decodeCursor(String value) {
    try {
      final decoded = utf8.decode(base64Url.decode(value));
      final separator = decoded.indexOf('\u0000');
      if (separator <= 0 || separator == decoded.length - 1) {
        throw const FormatException('invalid cursor');
      }
      final micros = int.parse(decoded.substring(0, separator));
      return (
        updatedAt: DateTime.fromMicrosecondsSinceEpoch(micros, isUtc: true),
        id: decoded.substring(separator + 1),
      );
    } on FormatException {
      rethrow;
    } catch (error) {
      throw FormatException('invalid cursor: $error');
    }
  }
}

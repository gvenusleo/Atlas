import 'dart:convert';
import 'dart:io';

import 'package:atlas_runtime/atlas_runtime.dart' as runtime;
import 'package:drift/drift.dart';
import 'package:drift/native.dart';

import '../database/database.dart';
import '../mappers/row_mappers.dart';

/// Drift-backed implementation of the runtime session persistence port.
final class DriftSessionStore implements runtime.SessionStore {
  /// Creates a store over an open Atlas database.
  DriftSessionStore(this.database, {RowMappers? mappers})
    : mappers = mappers ?? RowMappers();

  /// Opens a persistent store backed by [file].
  factory DriftSessionStore.openFile(File file, {RowMappers? mappers}) =>
      DriftSessionStore(AtlasDatabase.openFile(file), mappers: mappers);

  /// Creates an in-memory store for tests and local ephemeral sessions.
  factory DriftSessionStore.inMemory({RowMappers? mappers}) =>
      DriftSessionStore(
        AtlasDatabase(NativeDatabase.memory()),
        mappers: mappers,
      );

  /// The owned database connection.
  final AtlasDatabase database;

  /// The row/domain mappers.
  final RowMappers mappers;

  /// Closes the underlying database connection.
  Future<void> close() => database.close();

  @override
  Future<void> createSession(runtime.Session session) => database
      .into(database.sessions)
      .insert(mappers.sessionCompanion(session));

  @override
  Future<runtime.SessionSnapshot> loadSession(
    runtime.SessionId sessionId,
  ) async {
    final sessionRow = await _sessionRow(sessionId);
    final compactionRow =
        await (database.select(database.compactionCheckpoints)
              ..where((table) => table.sessionId.equals(sessionId.value)))
            .getSingleOrNull();
    final turnRows =
        await (database.select(database.turns)
              ..where((table) => table.sessionId.equals(sessionId.value))
              ..orderBy([(table) => OrderingTerm.asc(table.startedAt)]))
            .get();
    final timelineRows =
        await (database.select(database.timelineItems)
              ..where((table) => table.sessionId.equals(sessionId.value))
              ..orderBy([(table) => OrderingTerm.asc(table.sequence)]))
            .get();
    final checkpointRows = timelineRows.isEmpty
        ? const <ModelCheckpointRow>[]
        : await (database.select(database.modelCheckpoints)..where(
                (table) => table.timelineItemId.isIn(
                  timelineRows.map((row) => row.id),
                ),
              ))
              .get();
    final checkpointsByItem = {
      for (final row in checkpointRows) row.timelineItemId: row,
    };
    return runtime.SessionSnapshot(
      session: mappers.session(
        sessionRow,
        compaction: compactionRow == null
            ? null
            : mappers.compaction(compactionRow),
      ),
      turns: List<runtime.Turn>.unmodifiable(turnRows.map(mappers.turn)),
      timeline: List<runtime.TimelineItem>.unmodifiable(
        timelineRows.map(mappers.timelineItem),
      ),
      modelCheckpoints: List<runtime.ModelCheckpoint>.unmodifiable(
        timelineRows
            .map((row) => checkpointsByItem[row.id])
            .nonNulls
            .map(mappers.modelCheckpoint),
      ),
    );
  }

  @override
  Future<runtime.SessionPage> listSessions(runtime.SessionQuery query) async {
    final limit = query.limit <= 0 ? 20 : query.limit.clamp(1, 100);
    final cursor = query.cursor == null ? null : _decodeCursor(query.cursor!);
    final select = database.select(database.sessions);
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
      pageRows.map(mappers.sessionSummary),
    );
    return runtime.SessionPage(
      items: items,
      nextCursor: hasMore && pageRows.isNotEmpty
          ? _encodeCursor(pageRows.last.updatedAt, pageRows.last.id)
          : null,
    );
  }

  @override
  Future<void> beginTurn(runtime.BeginTurn operation) => database.transaction(
    () async {
      _validateBeginTurn(operation);
      await database
          .into(database.sessions)
          .insertOnConflictUpdate(mappers.sessionCompanion(operation.session));
      await database
          .into(database.turns)
          .insert(mappers.turnCompanion(operation.turn));
      await _insertTimeline(operation.userMessage);
      final title = operation.session.title.isEmpty
          ? _title(operation.userMessage)
          : operation.session.title;
      await (database.update(
        database.sessions,
      )..where((table) => table.id.equals(operation.session.id.value))).write(
        SessionsCompanion(
          title: Value(title),
          updatedAt: Value(operation.userMessage.occurredAt.toUtc()),
        ),
      );
    },
  );

  @override
  Future<void> appendModelStep(
    runtime.SessionId sessionId,
    runtime.PersistedModelStep operation,
  ) => database.transaction(() async {
    await _requireTimelineOwnership(operation.assistantMessage, sessionId);
    await _insertTimeline(operation.assistantMessage);
    for (final call in operation.toolCalls) {
      await _requireTimelineOwnership(call, sessionId);
      await _insertTimeline(call);
    }
    final checkpoint = operation.checkpoint;
    if (checkpoint != null) {
      if (checkpoint.timelineItemId != operation.assistantMessage.id) {
        throw const FormatException(
          'model checkpoint must reference its assistant item',
        );
      }
      await database
          .into(database.modelCheckpoints)
          .insert(mappers.modelCheckpointCompanion(checkpoint));
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
  ) => database.transaction(() async {
    await _requireTimelineOwnership(item, sessionId);
    await _insertTimeline(item);
    await _touchSession(sessionId, item.occurredAt);
  });

  @override
  Future<void> finishTurn(runtime.SessionId sessionId, runtime.Turn turn) =>
      database.transaction(() async {
        if (turn.sessionId != sessionId ||
            turn.status == runtime.TurnStatus.running) {
          throw const FormatException(
            'turn must be terminal and match the session',
          );
        }
        final count =
            await (database.update(database.turns)..where(
                  (table) =>
                      table.id.equals(turn.id.value) &
                      table.sessionId.equals(sessionId.value),
                ))
                .write(mappers.turnCompanion(turn));
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
  ) => database.transaction(() async {
    if (checkpoint.sessionId != sessionId) {
      throw const FormatException('compaction must match the session');
    }
    await database
        .into(database.compactionCheckpoints)
        .insertOnConflictUpdate(mappers.compactionCompanion(checkpoint));
    await _touchSession(sessionId, checkpoint.createdAt);
  });

  @override
  Future<void> deleteSession(runtime.SessionId sessionId) async {
    final count = await (database.delete(
      database.sessions,
    )..where((table) => table.id.equals(sessionId.value))).go();
    if (count == 0) {
      throw runtime.SessionNotFoundException(sessionId);
    }
  }

  Future<SessionRow> _sessionRow(runtime.SessionId sessionId) async {
    final row = await (database.select(
      database.sessions,
    )..where((table) => table.id.equals(sessionId.value))).getSingleOrNull();
    if (row == null) {
      throw runtime.SessionNotFoundException(sessionId);
    }
    return row;
  }

  Future<void> _insertTimeline(runtime.TimelineItem item) => database
      .into(database.timelineItems)
      .insert(mappers.timelineCompanion(item));

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
    final count = await (database.update(
      database.sessions,
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
    final turn = await (database.select(
      database.turns,
    )..where((table) => table.id.equals(item.turnId.value))).getSingleOrNull();
    if (turn == null || turn.sessionId != sessionId.value) {
      throw const FormatException('timeline item turn must match the session');
    }
  }

  static String _title(runtime.UserMessageItem item) {
    final text = runtime.textFromContent(item.content).trim();
    if (text.isEmpty) {
      return '';
    }
    final firstLine = text.split('\n').first.trim();
    final runes = firstLine.runes.toList();
    return String.fromCharCodes(runes.take(80));
  }

  static String _encodeCursor(DateTime updatedAt, String id) =>
      base64Url.encode(
        utf8.encode('${updatedAt.toUtc().microsecondsSinceEpoch}\u0000$id'),
      );

  static _SessionCursor _decodeCursor(String value) {
    try {
      final decoded = utf8.decode(base64Url.decode(value));
      final separator = decoded.indexOf('\u0000');
      if (separator <= 0 || separator == decoded.length - 1) {
        throw const FormatException('invalid cursor');
      }
      final micros = int.parse(decoded.substring(0, separator));
      return _SessionCursor(
        DateTime.fromMicrosecondsSinceEpoch(micros, isUtc: true),
        decoded.substring(separator + 1),
      );
    } on FormatException {
      rethrow;
    } catch (error) {
      throw FormatException('invalid cursor: $error');
    }
  }
}

final class _SessionCursor {
  const _SessionCursor(this.updatedAt, this.id);

  final DateTime updatedAt;
  final String id;
}

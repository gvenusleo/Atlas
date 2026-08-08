import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart' as runtime;
import 'package:drift/drift.dart';

import '../database/database.dart';
import 'timeline_codec.dart';

/// Converts Drift rows and companions at the storage boundary.
final class RowMappers {
  final TimelineCodec _timelineCodec = TimelineCodec();

  /// Converts a session row into a runtime session.
  runtime.Session session(
    SessionRow row, {
    runtime.CompactionCheckpoint? compaction,
  }) => runtime.Session(
    id: runtime.SessionId(row.id),
    title: row.title,
    workingDirectory: row.workingDirectory,
    additionalDirectories: _decodeDirectories(row.additionalDirectoriesJson),
    createdAt: row.createdAt.toUtc(),
    updatedAt: row.updatedAt.toUtc(),
    compaction: compaction,
    lastUsage: _sessionUsage(row),
  );

  /// Converts a session row into a list summary.
  runtime.SessionSummary sessionSummary(SessionRow row) =>
      runtime.SessionSummary(
        id: runtime.SessionId(row.id),
        title: row.title,
        workingDirectory: row.workingDirectory,
        updatedAt: row.updatedAt.toUtc(),
        lastUsage: _sessionUsage(row),
      );

  /// Converts a runtime session into a Drift companion.
  SessionsCompanion sessionCompanion(runtime.Session value) =>
      SessionsCompanion.insert(
        id: value.id.value,
        title: Value(value.title),
        workingDirectory: value.workingDirectory,
        additionalDirectoriesJson: Value(
          jsonEncode(value.additionalDirectories),
        ),
        lastInputTokens: Value(value.lastUsage.inputTokens),
        lastOutputTokens: Value(value.lastUsage.outputTokens),
        lastTotalTokens: Value(value.lastUsage.totalTokens),
        lastCacheReadTokens: Value(value.lastUsage.cacheReadInputTokens),
        lastCacheWriteTokens: Value(value.lastUsage.cacheWriteInputTokens),
        createdAt: value.createdAt.toUtc(),
        updatedAt: value.updatedAt.toUtc(),
      );

  /// Converts a turn row into a runtime turn.
  runtime.Turn turn(TurnRow row) => runtime.Turn(
    id: runtime.TurnId(row.id),
    sessionId: runtime.SessionId(row.sessionId),
    status: _enumByName(runtime.TurnStatus.values, row.status, 'turn status'),
    startedAt: row.startedAt.toUtc(),
    completedAt: row.completedAt?.toUtc(),
    model: row.providerId == null || row.modelId == null
        ? null
        : runtime.ModelRef(
            providerId: runtime.ProviderId(row.providerId!),
            modelId: runtime.ModelId(row.modelId!),
          ),
    reasoningEffort: row.reasoningEffort,
    usage: runtime.TokenUsage(
      inputTokens: row.inputTokens,
      outputTokens: row.outputTokens,
      totalTokens: row.totalTokens,
      cacheReadInputTokens: row.cacheReadTokens,
      cacheWriteInputTokens: row.cacheWriteTokens,
    ),
    failure: row.failureCode == null || row.failureMessage == null
        ? null
        : runtime.TurnFailure(
            code: row.failureCode!,
            message: row.failureMessage!,
          ),
    cancelReason: row.cancelReason,
  );

  /// Converts a runtime turn into a Drift companion.
  TurnsCompanion turnCompanion(runtime.Turn value) => TurnsCompanion.insert(
    id: value.id.value,
    sessionId: value.sessionId.value,
    status: value.status.name,
    startedAt: value.startedAt.toUtc(),
    completedAt: Value(value.completedAt?.toUtc()),
    providerId: Value(value.model?.providerId.value),
    modelId: Value(value.model?.modelId.value),
    reasoningEffort: Value(value.reasoningEffort),
    inputTokens: Value(value.usage.inputTokens),
    outputTokens: Value(value.usage.outputTokens),
    totalTokens: Value(value.usage.totalTokens),
    cacheReadTokens: Value(value.usage.cacheReadInputTokens),
    cacheWriteTokens: Value(value.usage.cacheWriteInputTokens),
    failureCode: Value(value.failure?.code),
    failureMessage: Value(value.failure?.message),
    cancelReason: Value(value.cancelReason),
  );

  /// Converts a timeline row into a runtime variant.
  runtime.TimelineItem timelineItem(TimelineItemRow row) =>
      _timelineCodec.decode(
        id: runtime.TimelineItemId(row.id),
        sessionId: runtime.SessionId(row.sessionId),
        turnId: runtime.TurnId(row.turnId),
        sequence: row.sequence,
        occurredAt: row.occurredAt.toUtc(),
        kind: row.kind,
        version: row.payloadVersion,
        payload: row.payloadJson,
      );

  /// Converts a runtime timeline item into a Drift companion.
  TimelineItemsCompanion timelineCompanion(runtime.TimelineItem value) {
    final encoded = _timelineCodec.encode(value);
    return TimelineItemsCompanion.insert(
      id: value.id.value,
      sessionId: value.sessionId.value,
      turnId: value.turnId.value,
      sequence: value.sequence,
      kind: encoded.kind,
      payloadVersion: Value(encoded.version),
      payloadJson: encoded.payload,
      occurredAt: value.occurredAt.toUtc(),
    );
  }

  /// Converts a model checkpoint row into a runtime checkpoint.
  runtime.ModelCheckpoint modelCheckpoint(ModelCheckpointRow row) =>
      runtime.ModelCheckpoint(
        timelineItemId: runtime.TimelineItemId(row.timelineItemId),
        continuation: runtime.ModelContinuation(
          providerId: runtime.ProviderId(row.providerId),
          reasoningSummary: row.reasoningSummary,
          opaquePayload: _decodeObject(row.payloadJson, 'model checkpoint'),
        ),
        createdAt: row.createdAt.toUtc(),
      );

  /// Converts a runtime model checkpoint into a Drift companion.
  ModelCheckpointsCompanion modelCheckpointCompanion(
    runtime.ModelCheckpoint value,
  ) => ModelCheckpointsCompanion.insert(
    timelineItemId: value.timelineItemId.value,
    providerId: value.continuation.providerId.value,
    reasoningSummary: Value(value.continuation.reasoningSummary),
    payloadJson: Value(jsonEncode(value.continuation.opaquePayload)),
    createdAt: value.createdAt.toUtc(),
  );

  /// Converts a compaction row into a runtime checkpoint.
  runtime.CompactionCheckpoint compaction(CompactionCheckpointRow row) =>
      runtime.CompactionCheckpoint(
        sessionId: runtime.SessionId(row.sessionId),
        compactedThroughSequence: row.compactedThroughSequence,
        summary: row.summary,
        inputTokensBefore: row.inputTokensBefore,
        inputTokensAfter: row.inputTokensAfter,
        createdAt: row.createdAt.toUtc(),
      );

  /// Converts a runtime compaction checkpoint into a Drift companion.
  CompactionCheckpointsCompanion compactionCompanion(
    runtime.CompactionCheckpoint value,
  ) => CompactionCheckpointsCompanion.insert(
    sessionId: value.sessionId.value,
    compactedThroughSequence: value.compactedThroughSequence,
    summary: value.summary,
    inputTokensBefore: value.inputTokensBefore,
    inputTokensAfter: value.inputTokensAfter,
    createdAt: value.createdAt.toUtc(),
  );

  static runtime.TokenUsage _sessionUsage(SessionRow row) => runtime.TokenUsage(
    inputTokens: row.lastInputTokens,
    outputTokens: row.lastOutputTokens,
    totalTokens: row.lastTotalTokens,
    cacheReadInputTokens: row.lastCacheReadTokens,
    cacheWriteInputTokens: row.lastCacheWriteTokens,
  );

  static List<String> _decodeDirectories(String value) {
    final decoded = jsonDecode(value);
    if (decoded is! List<Object?> || decoded.any((item) => item is! String)) {
      throw const FormatException(
        'additional directories must be a string array',
      );
    }
    return List<String>.unmodifiable(decoded.cast<String>());
  }

  static runtime.JsonObject _decodeObject(String value, String label) {
    final decoded = jsonDecode(value);
    if (decoded is! Map<String, Object?>) {
      throw FormatException('$label must be a JSON object');
    }
    return runtime.immutableJsonObject(decoded);
  }

  static T _enumByName<T extends Enum>(
    List<T> values,
    String name,
    String label,
  ) {
    for (final value in values) {
      if (value.name == name) {
        return value;
      }
    }
    throw FormatException('Unsupported $label: $name');
  }
}

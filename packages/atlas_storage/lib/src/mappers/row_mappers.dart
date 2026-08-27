import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart' as runtime;
import 'package:drift/drift.dart';

import '../database/database.dart';
import 'timeline_codec.dart';

/// Converts Drift rows and companions at the storage boundary.
final class RowMappers {
  final TimelineCodec _timelineCodec = TimelineCodec();

  /// Converts a session row into a runtime session.
  runtime.Session session(SessionRow row) => runtime.Session(
    id: runtime.SessionId(row.id),
    title: row.title,
    workingDirectory: row.workingDirectory,
    model: row.modelProviderId == null || row.modelId == null
        ? null
        : runtime.ModelRef(
            providerId: runtime.ProviderId(row.modelProviderId!),
            modelId: runtime.ModelId(row.modelId!),
          ),
    reasoningEffort: row.reasoningEffort,
    additionalDirectories: _decodeDirectories(row.additionalDirectoriesJson),
    createdAt: row.createdAt.toUtc(),
    updatedAt: row.updatedAt.toUtc(),
    compaction: _sessionCompaction(row),
    lastUsage: _sessionUsage(row),
  );

  /// Converts a session row into a list summary.
  runtime.SessionSummary sessionSummary(SessionRow row) =>
      runtime.SessionSummary(
        id: runtime.SessionId(row.id),
        title: row.title,
        workingDirectory: row.workingDirectory,
        model: row.modelProviderId == null || row.modelId == null
            ? null
            : runtime.ModelRef(
                providerId: runtime.ProviderId(row.modelProviderId!),
                modelId: runtime.ModelId(row.modelId!),
              ),
        reasoningEffort: row.reasoningEffort,
        additionalDirectories: _decodeDirectories(
          row.additionalDirectoriesJson,
        ),
        updatedAt: row.updatedAt.toUtc(),
        lastUsage: _sessionUsage(row),
      );

  /// Converts a runtime session into a Drift companion.
  SessionsCompanion sessionCompanion(runtime.Session value) =>
      SessionsCompanion.insert(
        id: value.id.value,
        title: Value(value.title),
        workingDirectory: value.workingDirectory,
        modelProviderId: Value(value.model?.providerId.value),
        modelId: Value(value.model?.modelId.value),
        reasoningEffort: Value(value.reasoningEffort),
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
            kind: row.failureKind ?? 'internal',
            providerDetail: row.providerDetail,
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
    failureKind: Value(value.failure?.kind),
    failureMessage: Value(value.failure?.message),
    providerDetail: Value(value.failure?.providerDetail),
    cancelReason: Value(value.cancelReason),
  );

  /// Converts a message row into a runtime variant and its continuation.
  ({runtime.TimelineItem item, runtime.ModelCheckpoint? checkpoint}) message(
    MessageRow row,
  ) => _timelineCodec.decode(
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
  MessagesCompanion messageCompanion(
    runtime.TimelineItem value, {
    runtime.ModelCheckpoint? checkpoint,
  }) {
    final encoded = _timelineCodec.encode(value, checkpoint: checkpoint);
    return MessagesCompanion.insert(
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

  static runtime.CompactionCheckpoint? _sessionCompaction(SessionRow row) {
    final createdAt = row.compactionCreatedAt;
    if (createdAt == null) {
      return null;
    }
    return runtime.CompactionCheckpoint(
      sessionId: runtime.SessionId(row.id),
      compactedThroughSequence: row.compactionSequence ?? 0,
      summary: row.compactionSummary,
      keptRecentMessages: row.compactionKeptRecent,
      inputTokensBefore: row.compactionTokensBefore,
      inputTokensAfter: row.compactionTokensAfter,
      createdAt: createdAt.toUtc(),
    );
  }

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

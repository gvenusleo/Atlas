import 'content.dart';
import 'ids.dart';
import 'model.dart';
import 'usage.dart';
import '../ports/cancellation.dart';

/// The lifecycle state of a user turn.
enum TurnStatus {
  /// The turn is executing.
  running,

  /// The turn reached a final assistant response.
  completed,

  /// The turn failed before completion.
  failed,

  /// The caller cancelled the turn after it started.
  cancelled,
}

/// A structured failure recorded for a turn.
final class TurnFailure {
  /// Creates a turn failure.
  const TurnFailure({required this.code, required this.message});

  /// A stable runtime error code.
  final String code;

  /// A user-visible error message.
  final String message;
}

/// The request that starts one user turn.
final class TurnRequest {
  /// Creates a turn request.
  const TurnRequest({
    required this.content,
    this.sessionId,
    this.model,
    this.reasoningEffort,
    this.workingDirectory,
    this.additionalDirectories,
    this.skills = const <String>[],
    this.cancellation,
  });

  /// The raw user content submitted for this turn.
  final List<ContentPart> content;

  /// An existing session to resume, or null to create one.
  final SessionId? sessionId;

  /// The requested model override.
  final ModelRef? model;

  /// The requested reasoning effort value.
  final String? reasoningEffort;

  /// The working directory for a new session; ignored when [sessionId]
  /// resumes an existing session, which keeps its own directory.
  final String? workingDirectory;

  /// Additional tool-accessible roots, or null to preserve session roots.
  final List<String>? additionalDirectories;

  /// Explicitly selected skill names whose full instructions are injected
  /// into this turn as non-persistent context.
  final List<String> skills;

  /// Cooperative cancellation for the turn.
  final CancellationToken? cancellation;
}

/// The result emitted when a turn reaches a terminal state.
final class TurnOutcome {
  /// Creates a turn outcome.
  const TurnOutcome({
    required this.sessionId,
    required this.turnId,
    required this.status,
    this.content = const <ContentPart>[],
    this.usage = const TokenUsage(),
    this.stopReason,
    this.failure,
  });

  /// The session that contains the turn.
  final SessionId sessionId;

  /// The completed turn.
  final TurnId turnId;

  /// The terminal turn status.
  final TurnStatus status;

  /// The final assistant content, when available.
  final List<ContentPart> content;

  /// The latest model usage.
  final TokenUsage usage;

  /// Why the terminal model step stopped, for completed turns.
  final StopReason? stopReason;

  /// The failure when the turn did not complete successfully.
  final TurnFailure? failure;
}

/// A durable turn record.
final class Turn {
  /// Creates a turn record.
  const Turn({
    required this.id,
    required this.sessionId,
    required this.status,
    required this.startedAt,
    this.completedAt,
    this.model,
    this.reasoningEffort,
    this.usage = const TokenUsage(),
    this.failure,
    this.cancelReason,
  });

  /// The turn identifier.
  final TurnId id;

  /// The owning session identifier.
  final SessionId sessionId;

  /// The lifecycle status.
  final TurnStatus status;

  /// The UTC start time.
  final DateTime startedAt;

  /// The UTC terminal time.
  final DateTime? completedAt;

  /// The model selected for this turn.
  final ModelRef? model;

  /// The reasoning effort selected for this turn.
  final String? reasoningEffort;

  /// The latest accumulated usage.
  final TokenUsage usage;

  /// The failure for failed turns.
  final TurnFailure? failure;

  /// The cancellation reason for cancelled turns.
  final String? cancelReason;
}

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
final class const TurnFailure({
  /// A stable runtime error code.
  required final String code,

  /// A user-visible error message.
  required final String message,

  /// Stable failure category used for retry and diagnostics.
  final String kind = 'internal',

  /// Bounded provider diagnostic detail, when available.
  final String? providerDetail,
}) {
  /// Creates a turn failure.
  this;
}

/// The request that starts one user turn.
final class const TurnRequest({
  /// The raw user content submitted for this turn.
  required final List<ContentPart> content,

  /// An existing session to resume, or null to create one.
  final SessionId? sessionId,

  /// The requested model override.
  final ModelRef? model,

  /// The requested reasoning effort value.
  final String? reasoningEffort,

  /// The requested agent session mode, applied before the prompt.
  final String? mode,

  /// The working directory for a new session; ignored when [sessionId]
  /// resumes an existing session, which keeps its own directory.
  final String? workingDirectory,

  /// Additional tool-accessible roots, or null to preserve session roots.
  final List<String>? additionalDirectories,

  /// Explicitly selected skill names whose full instructions are injected
  /// into this turn as non-persistent context.
  final List<String> skills = const <String>[],

  /// Cooperative cancellation for the turn.
  final CancellationToken? cancellation,
}) {
  /// Creates a turn request.
  this;
}

/// The result emitted when a turn reaches a terminal state.
final class const TurnOutcome({
  /// The session that contains the turn.
  required final SessionId sessionId,

  /// The completed turn.
  required final TurnId turnId,

  /// The terminal turn status.
  required final TurnStatus status,

  /// The final assistant content, when available.
  final List<ContentPart> content = const <ContentPart>[],

  /// The latest model usage.
  final TokenUsage usage = const TokenUsage(),

  /// Why the terminal model step stopped, for completed turns.
  final StopReason? stopReason,

  /// The failure when the turn did not complete successfully.
  final TurnFailure? failure,
}) {
  /// Creates a turn outcome.
  this;
}

/// A durable turn record.
final class const Turn({
  /// The turn identifier.
  required final TurnId id,

  /// The owning session identifier.
  required final SessionId sessionId,

  /// The lifecycle status.
  required final TurnStatus status,

  /// The UTC start time.
  required final DateTime startedAt,

  /// The UTC terminal time.
  final DateTime? completedAt,

  /// The model selected for this turn.
  final ModelRef? model,

  /// The reasoning effort selected for this turn.
  final String? reasoningEffort,

  /// The latest accumulated usage.
  final TokenUsage usage = const TokenUsage(),

  /// The failure for failed turns.
  final TurnFailure? failure,

  /// The cancellation reason for cancelled turns.
  final String? cancelReason,
}) {
  /// Creates a turn record.
  this;
}

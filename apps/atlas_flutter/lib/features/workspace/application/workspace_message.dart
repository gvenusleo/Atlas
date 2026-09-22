import 'package:atlas_runtime/atlas_runtime.dart';

/// Message kinds rendered in the Flutter conversation timeline.
enum WorkspaceLocalMessage {
  cannotLoadSessions,
  directorySaveFailed,
  cannotResumeSession,
  cannotRenameSession,
  cannotDeleteSession,
  cannotSetMode,
  slashCommandsNoImages,
  modelImagesOmitted,
  modelImageInputUnsupported,
  chooseRemoteDirectoryFirst,
  turnCancelled,
  turnFailed,
  noSessionToCompact,
  compactionFailed,
  contextCompacted,
}

String workspaceLocalMessageEnglish(
  WorkspaceLocalMessage message,
  List<Object> arguments,
) {
  final error = arguments.isEmpty ? '' : '${arguments.first}';
  return switch (message) {
    WorkspaceLocalMessage.cannotLoadSessions => 'Cannot load sessions: $error',
    WorkspaceLocalMessage.directorySaveFailed =>
      'The directory is active but could not be saved for the next connection.',
    WorkspaceLocalMessage.cannotResumeSession =>
      'Cannot resume session: $error',
    WorkspaceLocalMessage.cannotRenameSession =>
      'Cannot rename session: $error',
    WorkspaceLocalMessage.cannotDeleteSession =>
      'Cannot delete session: $error',
    WorkspaceLocalMessage.cannotSetMode => 'Cannot set mode: $error',
    WorkspaceLocalMessage.slashCommandsNoImages =>
      'Slash commands do not support images.',
    WorkspaceLocalMessage.modelImagesOmitted =>
      '${arguments.first} does not support images; images in this conversation will be omitted.',
    WorkspaceLocalMessage.modelImageInputUnsupported =>
      '${arguments.first} does not support image input.',
    WorkspaceLocalMessage.chooseRemoteDirectoryFirst => 'Choose the working directory on the computer before sending the first message.',
    WorkspaceLocalMessage.turnCancelled => 'Turn cancelled',
    WorkspaceLocalMessage.turnFailed => 'Turn failed: $error',
    WorkspaceLocalMessage.noSessionToCompact => 'No session to compact.',
    WorkspaceLocalMessage.compactionFailed => 'Compaction failed: $error',
    WorkspaceLocalMessage.contextCompacted =>
      'Context compacted, kept ${arguments.first} recent messages.',
  };
}

enum WorkspaceMessageKind {
  /// User-submitted text.
  user,

  /// Streaming or completed assistant Markdown.
  assistant,

  /// Streaming model reasoning summary.
  reasoning,

  /// A tool invocation and its result.
  tool,

  /// An agent plan with step statuses.
  plan,

  /// A local status notice.
  notice,

  /// A turn or loading failure.
  error,
}

/// One presentation-ready item in the conversation timeline.
final class const WorkspaceMessage({
  /// Stable identity used by the scrolling list.
  required final String id,

  /// The visual role of this item.
  required final WorkspaceMessageKind kind,

  /// Markdown text or tool output.
  required final String text,

  /// A local UI message resolved against the current locale at render time.
  final WorkspaceLocalMessage? localMessage,

  /// Values used by parameterized local messages.
  final List<Object> localArguments = const [],

  /// Data URLs or remote URIs for user-attached images.
  final List<String> imageSources = const [],

  /// Tool name for tool items.
  final String? toolName,

  /// Structured arguments for tool items.
  final JsonObject? arguments,

  /// When a tool item started, used to render its elapsed time.
  final DateTime? startedAt,

  /// Whether the item represents a failure.
  final bool isError = false,

  /// Whether a tool has not returned yet.
  final bool isRunning = false,
}) {
  /// Returns a copy with updated streaming or tool state.
  WorkspaceMessage copyWith({String? text, bool? isError, bool? isRunning}) =>
      WorkspaceMessage(
        id: id,
        kind: kind,
        text: text ?? this.text,
        localMessage: localMessage,
        localArguments: localArguments,
        imageSources: imageSources,
        toolName: toolName,
        arguments: arguments,
        startedAt: startedAt,
        isError: isError ?? this.isError,
        isRunning: isRunning ?? this.isRunning,
      );
}

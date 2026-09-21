import 'package:atlas_runtime/atlas_runtime.dart';

/// Message kinds rendered in the Flutter conversation timeline.
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
        imageSources: imageSources,
        toolName: toolName,
        arguments: arguments,
        startedAt: startedAt,
        isError: isError ?? this.isError,
        isRunning: isRunning ?? this.isRunning,
      );
}

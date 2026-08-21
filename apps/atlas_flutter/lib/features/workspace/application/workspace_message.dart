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

  /// A local status notice.
  notice,

  /// A turn or loading failure.
  error,
}

/// One presentation-ready item in the conversation timeline.
final class WorkspaceMessage {
  /// Creates a workspace message.
  const WorkspaceMessage({
    required this.id,
    required this.kind,
    required this.text,
    this.imageSources = const [],
    this.toolName,
    this.arguments,
    this.startedAt,
    this.isError = false,
    this.isRunning = false,
  });

  /// Stable identity used by the scrolling list.
  final String id;

  /// The visual role of this item.
  final WorkspaceMessageKind kind;

  /// Markdown text or tool output.
  final String text;

  /// Data URLs or remote URIs for user-attached images.
  final List<String> imageSources;

  /// Tool name for tool items.
  final String? toolName;

  /// Structured arguments for tool items.
  final JsonObject? arguments;

  /// When a tool item started, used to render its elapsed time.
  final DateTime? startedAt;

  /// Whether the item represents a failure.
  final bool isError;

  /// Whether a tool has not returned yet.
  final bool isRunning;

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

import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/timeline.dart';
import 'cancellation.dart';

/// A bounded replacement snapshot of output produced by a running tool.
final class ToolOutputSnapshot {
  /// Creates a transient output snapshot; this is not a persisted tool result.
  const ToolOutputSnapshot({
    required this.content,
    required this.totalBytes,
    required this.truncated,
  });

  /// Text replacing the previous output for this call.
  final String content;

  /// Raw output bytes observed before decoding and display filtering.
  final int totalBytes;

  /// Whether some displayable output was omitted.
  final bool truncated;
}

/// Context supplied to a tool invocation.
final class ToolContext {
  /// Creates a tool context.
  const ToolContext({
    required this.sessionId,
    required this.turnId,
    required this.workingDirectory,
    this.additionalDirectories = const <String>[],
    this.cancellation,
    this.onOutput,
  });

  /// The active session.
  final SessionId sessionId;

  /// The active turn.
  final TurnId turnId;

  /// The primary working directory.
  final String workingDirectory;

  /// Additional roots available to the tool.
  final List<String> additionalDirectories;

  /// Cooperative cancellation for the tool invocation.
  final CancellationToken? cancellation;

  /// Receives bounded replacement output while execution is in progress.
  final void Function(ToolOutputSnapshot snapshot)? onOutput;
}

/// A built-in or installed Atlas tool.
abstract interface class Tool {
  /// The model-facing descriptor.
  ToolDescriptor get descriptor;

  /// Executes the tool with parsed JSON arguments.
  Future<ToolResult> execute(ToolContext context, JsonObject arguments);
}

/// Resolves and executes model-requested tools.
abstract interface class ToolRegistry {
  /// Returns descriptors in stable registration order.
  List<ToolDescriptor> get descriptors;

  /// Executes a named tool.
  Future<ToolResult> execute(ToolContext context, ToolCall call);
}

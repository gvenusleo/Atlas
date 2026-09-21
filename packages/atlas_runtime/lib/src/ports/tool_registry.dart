import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/timeline.dart';
import 'cancellation.dart';

/// A bounded replacement snapshot of output produced by a running tool.
final class const ToolOutputSnapshot({
  /// Text replacing the previous output for this call.
  required final String content,

  /// Raw output bytes observed before decoding and display filtering.
  required final int totalBytes,

  /// Whether some displayable output was omitted.
  required final bool truncated,
}) {
  /// Creates a transient output snapshot; this is not a persisted tool result.
  this;
}

/// Context supplied to a tool invocation.
final class const ToolContext({
  /// The active session.
  required final SessionId sessionId,

  /// The active turn.
  required final TurnId turnId,

  /// The primary working directory.
  required final String workingDirectory,

  /// Additional roots available to the tool.
  final List<String> additionalDirectories = const <String>[],

  /// Cooperative cancellation for the tool invocation.
  final CancellationToken? cancellation,

  /// Receives bounded replacement output while execution is in progress.
  final void Function(ToolOutputSnapshot snapshot)? onOutput,
}) {
  /// Creates a tool context.
  this;
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

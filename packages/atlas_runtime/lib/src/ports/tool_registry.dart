import '../domain/ids.dart';
import '../domain/model.dart';
import '../domain/timeline.dart';
import 'cancellation.dart';

/// Context supplied to a tool invocation.
final class ToolContext {
  /// Creates a tool context.
  const ToolContext({
    required this.sessionId,
    required this.turnId,
    required this.workingDirectory,
    this.additionalDirectories = const <String>[],
    this.cancellation,
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

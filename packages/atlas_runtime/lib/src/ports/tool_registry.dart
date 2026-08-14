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
    this.fileReader,
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

  /// The client file reader, when the ACP client claims filesystem read
  /// support; null keeps tools on their local execution path.
  final ClientFileReader? fileReader;
}

/// The result of a delegated client file read.
final class ClientReadResult {
  /// Creates a client read result.
  const ClientReadResult({required this.content});

  /// The file contents as reported by the client.
  final String content;
}

/// Reads text files through the ACP client (for example to surface unsaved
/// editor state that the local filesystem cannot see).
///
/// Implemented by protocol adapters; runtime and tools only depend on this
/// port.
abstract interface class ClientFileReader {
  /// Reads [path] (already absolute), optionally starting at 1-indexed
  /// [line] with at most [limit] lines, and returns the client's contents.
  Future<ClientReadResult> readTextFile(
    SessionId sessionId, {
    required String path,
    int? line,
    int? limit,
  });
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

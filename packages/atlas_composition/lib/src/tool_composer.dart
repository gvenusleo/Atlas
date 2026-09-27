import 'dart:io';

import 'package:atlas_config/atlas_config.dart';
import 'package:atlas_mcp/atlas_mcp.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tools/atlas_tools.dart';

/// An immutable tool registry and the connections that it owns.
final class ComposedTools._(
  /// Registry supplied to the runtime and prompt builder.
  final ToolRegistry registry,
  final McpConnections _connections,
) {
  /// Creates the prepared tool set.
  this;

  /// Closes external connections after the runtime has drained its turns.
  Future<void> close() => _connections.close();
}

/// Returns the built-in tools in stable model-visible order.
List<Tool> builtInTools({Map<String, String>? environment}) => [
  ReadTool(),
  WriteTool(),
  EditTool(),
  ShellTool(environment: environment),
  PlanTool(),
];

/// Discovers configured MCP tools before constructing a runtime.
Future<ComposedTools> composeTools(
  AtlasConfig config, {
  Map<String, String>? environment,
  String? workingDirectory,
  CancellationToken? cancellation,
  AtlasLogger logger = const NoopLogger(),
}) async {
  final snapshot = Map<String, String>.unmodifiable(
    environment ?? Platform.environment,
  );
  final cwd = workingDirectory ?? Directory.current.path;
  final connections = await McpConnections.connect(
    [
      for (final server in config.mcpServers)
        if (server.enabled)
          switch (server) {
            McpStdioConfig() => McpStdioOptions(
              name: server.name,
              startupTimeout: server.startupTimeout,
              callTimeout: server.callTimeout,
              command: server.command,
              args: server.args,
              workingDirectory: server.workingDirectory ?? cwd,
              environment: Map.unmodifiable({
                ...snapshot,
                ...server.environment,
              }),
            ),
            McpHttpConfig() => McpHttpOptions(
              name: server.name,
              startupTimeout: server.startupTimeout,
              callTimeout: server.callTimeout,
              url: server.url,
              headers: server.headers,
            ),
          },
    ],
    cancellation: cancellation,
    logger: logger,
  );
  try {
    return ComposedTools._(
      LocalToolRegistry([
        ...builtInTools(environment: snapshot),
        ...connections.tools,
      ]),
      connections,
    );
  } catch (_) {
    await connections.close();
    rethrow;
  }
}

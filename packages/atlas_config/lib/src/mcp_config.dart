/// Configuration shared by MCP server transports.
sealed class const McpServerConfig({
  /// Stable server identifier.
  required final String name,

  /// Whether this server participates in startup.
  final bool enabled = true,

  /// Deadline for connecting and discovering all tools.
  final Duration startupTimeout = const Duration(seconds: 15),

  /// Total deadline for one tool invocation.
  final Duration callTimeout = const Duration(seconds: 60),
}) {
  /// Creates server configuration.
  this;
}

/// A local MCP subprocess.
final class const McpStdioConfig({
  required super.name,
  super.enabled,
  super.startupTimeout,
  super.callTimeout,

  /// Executable launched without a shell.
  required final String command,

  /// Literal executable arguments.
  final List<String> args = const [],

  /// Absolute working directory; null uses the Atlas startup directory.
  final String? workingDirectory,

  /// Overrides applied to the bootstrap environment snapshot.
  final Map<String, String> environment = const {},
}) extends McpServerConfig {
  /// Creates stdio configuration.
  this;
}

/// A remote MCP endpoint using Streamable HTTP.
final class const McpHttpConfig({
  required super.name,
  super.enabled,
  super.startupTimeout,
  super.callTimeout,

  /// Full endpoint URI.
  required final Uri url,

  /// Static headers, including optional authentication.
  final Map<String, String> headers = const {},
}) extends McpServerConfig {
  /// Creates HTTP configuration.
  this;
}

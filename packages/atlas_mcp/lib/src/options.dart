/// Connection settings independent of configuration-file parsing.
sealed class const McpServerOptions({
  /// Stable configured identity.
  required final String name,

  /// Total connection and discovery deadline.
  final Duration startupTimeout = const Duration(seconds: 15),

  /// Total deadline for one invocation.
  final Duration callTimeout = const Duration(seconds: 60),
}) {
  /// Creates connection settings.
  this;
}

/// Settings for a local subprocess, launched without a shell.
final class const McpStdioOptions({
  required super.name,
  super.startupTimeout,
  super.callTimeout,

  /// Executable name or path.
  required final String command,

  /// Literal executable arguments.
  final List<String> args = const [],

  /// The complete environment snapshot for the child.
  required final Map<String, String> environment,

  /// Absolute directory used throughout the connection's lifetime.
  required final String workingDirectory,
}) extends McpServerOptions {
  /// Creates subprocess settings.
  this;
}

/// Settings for Streamable HTTP with optional static authentication.
final class const McpHttpOptions({
  required super.name,
  super.startupTimeout,
  super.callTimeout,

  /// Complete endpoint URL.
  required final Uri url,

  /// Static HTTP headers.
  final Map<String, String> headers = const {},
}) extends McpServerOptions {
  /// Creates remote connection settings.
  this;
}

import 'package:atlas_runtime/atlas_runtime.dart';

/// Runtime services and catalogs injected into Flutter presentation code.
final class const RuntimeEnvironment({
  /// The single runtime used by every Flutter feature.
  required final PresentationAgentSession runtime,

  /// Models configured for user selection.
  required final List<ModelDescriptor> models,

  /// Closes process-owned resources when the application exits.
  final Future<void> Function()? onClose,

  /// Whether the runtime is served by a remote `atlas server` over
  /// WebSocket; remote sessions cannot browse or run local terminals.
  final bool isRemote = false,

  /// Completes when the underlying connection ends, for remote runtimes.
  final Future<void>? closed,
}) {
  /// Creates an environment around the shared agent runtime.
  this;

  /// Releases resources owned by this application environment.
  Future<void> close() async => onClose?.call();
}

/// Result of loading the local Atlas configuration and composing the runtime.
final class RuntimeBootstrap {
  /// Creates a successful bootstrap result.
  const new ready(this.environment) : error = null;

  /// Creates a failed bootstrap result that keeps the application usable.
  const new failed(this.error) : environment = null;

  /// The composed runtime environment when configuration succeeded.
  final RuntimeEnvironment? environment;

  /// A user-visible startup failure when configuration did not succeed.
  final String? error;
}

/// A connected remote session and its disconnect notification.
final class const RemoteSessionHandle({
  /// The runtime environment backed by the remote server.
  required final RuntimeEnvironment environment,

  /// Completes when the underlying connection ends.
  required final Future<void> closed,
});

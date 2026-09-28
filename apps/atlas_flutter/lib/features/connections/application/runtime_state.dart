import 'package:atlas_flutter/features/connections/domain/acp_connection.dart';
import 'package:atlas_flutter/features/connections/domain/remote_connection_profile.dart';
import 'package:atlas_flutter/features/connections/domain/runtime_environment.dart';

/// The lifecycle state of a local ACP subprocess connection.
enum AcpConnectionStatus {
  /// No ACP connection is active; the local runtime is in use.
  disconnected,

  /// An ACP server process is starting.
  connecting,

  /// An ACP server is active.
  connected,

  /// The last activation attempt failed.
  error,
}

/// The lifecycle state of a remote WebSocket connection.
enum RemoteConnectionStatus {
  /// No remote connection is active.
  disconnected,

  /// The first connection attempt is running.
  connecting,

  /// The remote connection is active.
  connected,

  /// The connection dropped and automatic reconnection is in progress.
  reconnecting,

  /// The last connection attempt failed and the profile stays editable.
  error,
}

/// The active local and remote connection lifecycle snapshot.
final class const AcpRuntimeState({
  /// The active runtime environment, or null on startup failure.
  required final RuntimeEnvironment? environment,

  /// The local ACP connection lifecycle state.
  final AcpConnectionStatus status = AcpConnectionStatus.disconnected,

  /// The last local ACP activation error, if any.
  final String? activationError,

  /// The local ACP connection currently in use, or null for the local
  /// runtime.
  final AcpConnection? activeConnection,

  /// The remote profile currently in use, or null when none is active.
  final RemoteConnectionProfile? remoteProfile,

  /// The remote WebSocket connection lifecycle state.
  final RemoteConnectionStatus remoteStatus =
      RemoteConnectionStatus.disconnected,

  /// The last remote connection error, if any.
  final String? remoteError,
});

import 'package:atlas_flutter/features/connections/domain/acp_connection.dart';

/// A configured remote Atlas server connection.
///
/// Unlike [AcpConnection] (a local ACP subprocess), this profile describes a
/// WebSocket endpoint: [wsUrl] is the `ws(s)://host/acp` address and [token]
/// is the bearer token issued by `atlas server`. The token is persisted in
/// the platform secure storage, never in plain JSON.
final class const RemoteConnectionProfile({
  /// Display name, for example "My computer".
  required final String name,

  /// The WebSocket endpoint of the remote `atlas server`.
  required final String wsUrl,

  /// The bearer token guarding the endpoint.
  required final String token,

  /// Optional absolute working directory on the computer for new sessions.
  ///
  /// When null, the connection works without a directory: session history
  /// and model selection are available, and the app asks for the directory
  /// before the first message is sent (ACP requires a `cwd` on session
  /// creation).
  final String? workingDirectory,
}) {
  /// Copies this profile with the given fields replaced.
  RemoteConnectionProfile copyWith({String? workingDirectory}) {
    return RemoteConnectionProfile(
      name: name,
      wsUrl: wsUrl,
      token: token,
      workingDirectory: workingDirectory ?? this.workingDirectory,
    );
  }
}

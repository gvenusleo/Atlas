import 'package:atlas_flutter/features/connections/domain/remote_connection_profile.dart';

/// Converts persisted remote records to connection models.
abstract final class RemoteConnectionCodec {
  /// Encodes a profile without exposing its token to display or logging.
  static Map<String, Object?> encode(RemoteConnectionProfile profile) => {
    'name': profile.name,
    'wsUrl': profile.wsUrl,
    'workingDirectory': ?profile.workingDirectory,
  };

  /// Parses a profile from [json]; returns null when malformed.
  static RemoteConnectionProfile? decode(Object? json) {
    if (json case {'name': String name, 'wsUrl': String wsUrl}
        when name.isNotEmpty && wsUrl.isNotEmpty) {
      final token = json['token'];
      final workingDirectory = json['workingDirectory'];
      return RemoteConnectionProfile(
        name: name,
        wsUrl: wsUrl,
        token: token is String ? token : '',
        workingDirectory:
            workingDirectory is String && workingDirectory.isNotEmpty
            ? workingDirectory
            : null,
      );
    }
    return null;
  }
}

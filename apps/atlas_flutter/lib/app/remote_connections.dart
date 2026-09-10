import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// A configured remote Atlas server connection.
///
/// Unlike [AcpConnection] (a local ACP subprocess), this profile describes a
/// WebSocket endpoint: [wsUrl] is the `ws(s)://host/acp` address and [token]
/// is the bearer token issued by `atlas server`. The token is persisted in
/// the platform secure storage, never in plain JSON.
final class RemoteConnectionProfile {
  /// Creates a remote connection profile.
  const RemoteConnectionProfile({
    required this.name,
    required this.wsUrl,
    required this.token,
    this.workingDirectory,
  });

  /// Display name, for example "My computer".
  final String name;

  /// The WebSocket endpoint of the remote `atlas server`.
  final String wsUrl;

  /// The bearer token guarding the endpoint.
  final String token;

  /// Optional absolute working directory on the computer for new sessions.
  ///
  /// When null, the connection works without a directory: session history
  /// and model selection are available, and the app asks for the directory
  /// before the first message is sent (ACP requires a `cwd` on session
  /// creation).
  final String? workingDirectory;

  /// Copies this profile with the given fields replaced.
  RemoteConnectionProfile copyWith({String? workingDirectory}) {
    return RemoteConnectionProfile(
      name: name,
      wsUrl: wsUrl,
      token: token,
      workingDirectory: workingDirectory ?? this.workingDirectory,
    );
  }

  /// Serializes the profile without the token for display or logging.
  Map<String, Object?> toJson() => {
    'name': name,
    'wsUrl': wsUrl,
    'workingDirectory': ?workingDirectory,
  };

  /// Parses a profile from [json]; returns null when malformed.
  static RemoteConnectionProfile? fromJson(Object? json) {
    if (json is! Map) {
      return null;
    }
    final name = json['name'];
    final wsUrl = json['wsUrl'];
    if (name is! String || name.isEmpty || wsUrl is! String || wsUrl.isEmpty) {
      return null;
    }
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
}

/// Persists remote connection profiles in the platform secure storage.
///
/// The whole profile list (including tokens) is stored under one key in
/// Android Keystore / iOS Keychain backed storage so no plaintext token ever
/// reaches the filesystem.
final class RemoteConnectionStore {
  /// Creates a store over [storage].
  RemoteConnectionStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _profilesKey = 'atlas.remote_connections';

  final FlutterSecureStorage _storage;

  /// Loads saved profiles in display order; empty when none exist.
  Future<List<RemoteConnectionProfile>> load() async {
    final raw = await _storage.read(key: _profilesKey);
    if (raw == null || raw.isEmpty) {
      // Growable: callers merge edits into the returned list before saving.
      return <RemoteConnectionProfile>[];
    }
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! List) {
        return const [];
      }
      return [
        for (final entry in decoded) ?RemoteConnectionProfile.fromJson(entry),
      ];
    } on Object {
      return const [];
    }
  }

  /// Replaces the stored profile list with [profiles].
  Future<void> save(List<RemoteConnectionProfile> profiles) async {
    await _storage.write(
      key: _profilesKey,
      value: jsonEncode([
        for (final profile in profiles)
          {...profile.toJson(), 'token': profile.token},
      ]),
    );
  }
}

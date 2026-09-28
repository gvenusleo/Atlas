import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'package:atlas_flutter/features/connections/data/connection_repository.dart';
import 'package:atlas_flutter/features/connections/data/remote_connection_codec.dart';
import 'package:atlas_flutter/features/connections/domain/remote_connection_profile.dart';

/// Persists remote connection profiles in the platform secure storage.
///
/// The whole profile list (including tokens) is stored under one key in
/// Android Keystore / iOS Keychain backed storage so no plaintext token ever
/// reaches the filesystem.
final class RemoteConnectionStore({FlutterSecureStorage? storage})
    implements ConnectionStore<RemoteConnectionProfile> {
  /// Creates a store over [storage].
  this : _storage = storage ?? const FlutterSecureStorage();

  static const _profilesKey = 'atlas.remote_connections';

  final FlutterSecureStorage _storage;

  /// Loads saved profiles in display order; empty when none exist.
  @override
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
        for (final entry in decoded) ?RemoteConnectionCodec.decode(entry),
      ];
    } on Object {
      return const [];
    }
  }

  /// Replaces the stored profile list with [profiles].
  @override
  Future<void> save(List<RemoteConnectionProfile> profiles) async {
    await _storage.write(
      key: _profilesKey,
      value: jsonEncode([
        for (final profile in profiles)
          {...RemoteConnectionCodec.encode(profile), 'token': profile.token},
      ]),
    );
  }
}

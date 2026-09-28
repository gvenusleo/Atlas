import 'package:shared_preferences/shared_preferences.dart';

/// Platform storage for client-local preference values.
abstract interface class PreferencesStore {
  /// Reads a previously loaded string value.
  String? read(String key);

  /// Persists a string value for the next launch.
  Future<void> write(String key, String value);
}

/// Adapts the platform preference plugin to the settings data layer.
final class SharedPreferencesStore(SharedPreferences preferences)
    implements PreferencesStore {
  /// Creates a store over an already opened plugin instance.
  this : _preferences = preferences;

  final SharedPreferences _preferences;

  /// Opens the platform store; unavailable storage keeps preferences in memory.
  static Future<SharedPreferencesStore?> open() async {
    try {
      return SharedPreferencesStore(await SharedPreferences.getInstance());
    } on Object {
      return null;
    }
  }

  @override
  String? read(String key) => _preferences.getString(key);

  @override
  Future<void> write(String key, String value) async {
    await _preferences.setString(key, value);
  }
}

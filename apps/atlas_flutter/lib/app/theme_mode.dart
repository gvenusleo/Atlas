import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Preference key holding the client-local theme mode.
const themeModePreferenceKey = 'atlas.theme_mode';

/// Opens the client-local preference store, or returns null when the platform
/// store is unavailable.
///
/// A null store keeps the session working: the selection still applies for the
/// current run, only persistence is skipped.
Future<SharedPreferences?> openThemePreferences() async {
  try {
    return await SharedPreferences.getInstance();
  } on Object {
    return null;
  }
}

/// Reads the persisted theme mode, defaulting to [ThemeMode.system].
///
/// Values that are absent or no longer recognized fall back to the system
/// appearance instead of failing the launch.
ThemeMode loadThemeMode(SharedPreferences? preferences) {
  return switch (preferences?.getString(themeModePreferenceKey)) {
    'light' => ThemeMode.light,
    'dark' => ThemeMode.dark,
    _ => ThemeMode.system,
  };
}

/// Persists [mode] for the next launch.
Future<void> saveThemeMode(
  SharedPreferences? preferences,
  ThemeMode mode,
) async {
  if (preferences == null) {
    return;
  }
  await preferences.setString(themeModePreferenceKey, _encodeThemeMode(mode));
}

String _encodeThemeMode(ThemeMode mode) => switch (mode) {
  ThemeMode.light => 'light',
  ThemeMode.dark => 'dark',
  ThemeMode.system => 'system',
};

/// Holds the client-local theme preference for the whole application.
final themeModeProvider = NotifierProvider<ThemeModeController, ThemeMode>(
  ThemeModeController.new,
);

/// Applies and persists the client-local theme preference.
///
/// The bootstrap in `main` seeds the controller with the value read before the
/// first frame, so the first paint already uses the saved mode.
final class ThemeModeController extends Notifier<ThemeMode> {
  /// Creates a controller, optionally seeded with the persisted preference.
  ThemeModeController({
    ThemeMode initial = ThemeMode.system,
    SharedPreferences? preferences,
  }) : _initial = initial,
       _preferences = preferences;

  final ThemeMode _initial;
  final SharedPreferences? _preferences;

  @override
  ThemeMode build() => _initial;

  /// Selects [mode] and persists it for the next launch.
  Future<void> select(ThemeMode mode) async {
    state = mode;
    await saveThemeMode(_preferences, mode);
  }
}

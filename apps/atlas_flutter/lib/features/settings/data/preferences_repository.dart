import 'package:material_ui/material_ui.dart' show ThemeMode;

import 'package:atlas_flutter/features/settings/data/preferences_store.dart';
import 'package:atlas_flutter/features/settings/domain/app_language.dart';

/// Preference key holding the client-local theme mode.
const themeModePreferenceKey = 'atlas.theme_mode';

/// Preference key holding the client-local language.
const languagePreferenceKey = 'atlas.language';

/// Decodes settings and persists selections independently of platform plugins.
class const PreferencesRepository({PreferencesStore? store}) {
  final PreferencesStore? _store = store;

  /// Saved theme mode, falling back to the system for unknown values.
  ThemeMode get themeMode => switch (_store?.read(themeModePreferenceKey)) {
    'light' => ThemeMode.light,
    'dark' => ThemeMode.dark,
    _ => ThemeMode.system,
  };

  /// Saved language, falling back to the system for unknown values.
  AppLanguage get language => switch (_store?.read(languagePreferenceKey)) {
    'en' => AppLanguage.english,
    'zh-Hans' => AppLanguage.simplifiedChinese,
    _ => AppLanguage.system,
  };

  /// Persists the theme while allowing sessions without platform storage.
  Future<void> saveThemeMode(ThemeMode mode) async {
    await _store?.write(themeModePreferenceKey, switch (mode) {
      ThemeMode.system => 'system',
      ThemeMode.light => 'light',
      ThemeMode.dark => 'dark',
    });
  }

  /// Persists the language while allowing sessions without platform storage.
  Future<void> saveLanguage(AppLanguage language) async {
    await _store?.write(languagePreferenceKey, switch (language) {
      AppLanguage.system => 'system',
      AppLanguage.english => 'en',
      AppLanguage.simplifiedChinese => 'zh-Hans',
    });
  }
}

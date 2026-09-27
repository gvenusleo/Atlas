import 'package:atlas_flutter/app/theme_mode.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('reads stored modes and ignores unknown values', () async {
    SharedPreferences.setMockInitialValues({themeModePreferenceKey: 'dark'});
    expect(loadThemeMode(await openThemePreferences()), ThemeMode.dark);

    SharedPreferences.setMockInitialValues({themeModePreferenceKey: 'light'});
    expect(loadThemeMode(await openThemePreferences()), ThemeMode.light);

    SharedPreferences.setMockInitialValues({themeModePreferenceKey: 'moody'});
    expect(loadThemeMode(await openThemePreferences()), ThemeMode.system);
  });

  test('selecting a mode persists it for the next launch', () async {
    final preferences = await openThemePreferences();
    final container = ProviderContainer(
      overrides: [
        themeModeProvider.overrideWith(
          () => ThemeModeController(
            initial: loadThemeMode(preferences),
            preferences: preferences,
          ),
        ),
      ],
    );
    addTearDown(container.dispose);

    expect(container.read(themeModeProvider), ThemeMode.system);
    await container.read(themeModeProvider.notifier).select(ThemeMode.light);
    expect(container.read(themeModeProvider), ThemeMode.light);
    expect(preferences!.getString(themeModePreferenceKey), 'light');

    await container.read(themeModeProvider.notifier).select(ThemeMode.dark);
    expect(preferences.getString(themeModePreferenceKey), 'dark');
  });
}

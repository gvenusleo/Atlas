import 'package:atlas_flutter/features/settings/application/preferences_provider.dart';
import 'package:atlas_flutter/features/settings/application/theme_mode.dart';
import 'package:atlas_flutter/features/settings/data/preferences_repository.dart';
import 'package:atlas_flutter/features/settings/data/preferences_store.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('reads stored modes and ignores unknown values', () async {
    SharedPreferences.setMockInitialValues({themeModePreferenceKey: 'dark'});
    expect(
      PreferencesRepository(store: await SharedPreferencesStore.open())
          .themeMode,
      ThemeMode.dark,
    );

    SharedPreferences.setMockInitialValues({themeModePreferenceKey: 'light'});
    expect(
      PreferencesRepository(store: await SharedPreferencesStore.open())
          .themeMode,
      ThemeMode.light,
    );

    SharedPreferences.setMockInitialValues({themeModePreferenceKey: 'moody'});
    expect(
      PreferencesRepository(store: await SharedPreferencesStore.open())
          .themeMode,
      ThemeMode.system,
    );
  });

  test('selecting a mode persists it for the next launch', () async {
    final preferences = await SharedPreferences.getInstance();
    final container = ProviderContainer(
      overrides: [
        preferencesRepositoryProvider.overrideWithValue(
          PreferencesRepository(store: SharedPreferencesStore(preferences)),
        ),
      ],
    );
    addTearDown(container.dispose);

    expect(container.read(themeModeProvider), ThemeMode.system);
    await container.read(themeModeProvider.notifier).select(ThemeMode.light);
    expect(container.read(themeModeProvider), ThemeMode.light);
    expect(preferences.getString(themeModePreferenceKey), 'light');

    await container.read(themeModeProvider.notifier).select(ThemeMode.dark);
    expect(preferences.getString(themeModePreferenceKey), 'dark');
  });
}

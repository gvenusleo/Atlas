import 'package:atlas_flutter/features/settings/application/locale_mode.dart';
import 'package:atlas_flutter/features/settings/application/preferences_provider.dart';
import 'package:atlas_flutter/features/settings/data/preferences_repository.dart';
import 'package:atlas_flutter/features/settings/data/preferences_store.dart';
import 'package:atlas_flutter/features/settings/domain/app_language.dart';
import 'package:atlas_flutter/l10n/locale_resolution.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('selecting a language applies and persists it', () async {
    final preferences = await SharedPreferences.getInstance();
    final container = ProviderContainer(
      overrides: [
        preferencesRepositoryProvider.overrideWithValue(
          PreferencesRepository(store: SharedPreferencesStore(preferences)),
        ),
      ],
    );
    addTearDown(container.dispose);

    await container
        .read(languageProvider.notifier)
        .select(AppLanguage.simplifiedChinese);

    expect(container.read(languageProvider), AppLanguage.simplifiedChinese);
    expect(preferences.getString(languagePreferenceKey), 'zh-Hans');
    expect(
      localeForLanguage(AppLanguage.simplifiedChinese),
      const Locale.fromSubtags(languageCode: 'zh', scriptCode: 'Hans'),
    );
  });

  test('system locale resolution uses Simplified Chinese only', () {
    expect(
      resolveAtlasLocale(const Locale('zh', 'CN'), atlasSupportedLocales),
      simplifiedChineseLocale,
    );
    expect(
      resolveAtlasLocale(
        const Locale.fromSubtags(languageCode: 'zh', scriptCode: 'Hant'),
        atlasSupportedLocales,
      ),
      englishLocale,
    );
    expect(
      resolveAtlasLocale(const Locale('fr'), atlasSupportedLocales),
      englishLocale,
    );
  });

  test('keeps applying the language when persistence is unavailable', () async {
    final container = ProviderContainer(
      overrides: [
        languageProvider.overrideWith(
          () => LanguageController(initial: AppLanguage.english),
        ),
      ],
    );
    addTearDown(container.dispose);

    await container
        .read(languageProvider.notifier)
        .select(AppLanguage.simplifiedChinese);
    expect(container.read(languageProvider), AppLanguage.simplifiedChinese);
    await const PreferencesRepository().saveLanguage(AppLanguage.english);
  });
}

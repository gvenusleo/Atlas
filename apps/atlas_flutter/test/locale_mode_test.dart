import 'package:atlas_flutter/app/locale_mode.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('defaults to the system language without a stored value', () async {
    final preferences = await SharedPreferences.getInstance();

    expect(loadAppLanguage(preferences), AppLanguage.system);
    expect(loadAppLanguage(null), AppLanguage.system);
    expect(localeForLanguage(AppLanguage.system), isNull);
  });

  test('reads supported languages and ignores unknown values', () async {
    SharedPreferences.setMockInitialValues({languagePreferenceKey: 'en'});
    expect(
      loadAppLanguage(await SharedPreferences.getInstance()),
      AppLanguage.english,
    );

    SharedPreferences.setMockInitialValues({languagePreferenceKey: 'zh-Hans'});
    expect(
      loadAppLanguage(await SharedPreferences.getInstance()),
      AppLanguage.simplifiedChinese,
    );

    SharedPreferences.setMockInitialValues({languagePreferenceKey: 'fr'});
    expect(
      loadAppLanguage(await SharedPreferences.getInstance()),
      AppLanguage.system,
    );
  });

  test('selecting a language applies and persists it', () async {
    final preferences = await SharedPreferences.getInstance();
    final container = ProviderContainer(
      overrides: [
        languageProvider.overrideWith(
          () => LanguageController(preferences: preferences),
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
    await saveAppLanguage(null, AppLanguage.english);
  });
}

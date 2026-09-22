import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Client-local language selection. System leaves locale resolution to Flutter.
enum AppLanguage { system, english, simplifiedChinese }

const languagePreferenceKey = 'atlas.language';

AppLanguage loadAppLanguage(SharedPreferences? preferences) =>
    switch (preferences?.getString(languagePreferenceKey)) {
      'en' => AppLanguage.english,
      'zh-Hans' => AppLanguage.simplifiedChinese,
      _ => AppLanguage.system,
    };

Future<void> saveAppLanguage(
  SharedPreferences? preferences,
  AppLanguage language,
) async {
  if (preferences == null) return;
  await preferences.setString(languagePreferenceKey, switch (language) {
    AppLanguage.system => 'system',
    AppLanguage.english => 'en',
    AppLanguage.simplifiedChinese => 'zh-Hans',
  });
}

const englishLocale = Locale('en');
const simplifiedChineseLocale = Locale.fromSubtags(
  languageCode: 'zh',
  scriptCode: 'Hans',
);
const atlasSupportedLocales = [englishLocale, simplifiedChineseLocale];

Locale? localeForLanguage(AppLanguage language) => switch (language) {
  AppLanguage.system => null,
  AppLanguage.english => englishLocale,
  AppLanguage.simplifiedChinese => simplifiedChineseLocale,
};

/// Resolves system locales without treating Traditional Chinese as Simplified.
Locale resolveAtlasLocale(Locale? locale, Iterable<Locale> supportedLocales) {
  if (locale?.languageCode == 'zh') {
    final traditional =
        locale!.scriptCode == 'Hant' ||
        const {'TW', 'HK', 'MO'}.contains(locale.countryCode);
    if (!traditional) return simplifiedChineseLocale;
  }
  if (locale?.languageCode == 'en') return englishLocale;
  return englishLocale;
}

final languageProvider = NotifierProvider<LanguageController, AppLanguage>(
  LanguageController.new,
);

final class LanguageController({
  AppLanguage initial = AppLanguage.system,
  SharedPreferences? preferences,
}) extends Notifier<AppLanguage> {
  final AppLanguage _initial = initial;
  final SharedPreferences? _preferences = preferences;

  @override
  AppLanguage build() => _initial;

  Future<void> select(AppLanguage language) async {
    state = language;
    await saveAppLanguage(_preferences, language);
  }
}

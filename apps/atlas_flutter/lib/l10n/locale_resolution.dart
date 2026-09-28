import 'package:flutter/widgets.dart';

import 'package:atlas_flutter/features/settings/domain/app_language.dart';

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

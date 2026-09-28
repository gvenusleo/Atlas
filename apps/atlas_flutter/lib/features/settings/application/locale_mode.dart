import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:atlas_flutter/features/settings/application/preferences_provider.dart';
import 'package:atlas_flutter/features/settings/domain/app_language.dart';

/// Holds the client-local language preference for the whole application.
final languageProvider = NotifierProvider<LanguageController, AppLanguage>(
  LanguageController.new,
);

/// Applies language selections and delegates persistence to settings storage.
final class LanguageController({AppLanguage? initial})
    extends Notifier<AppLanguage> {
  final AppLanguage? _initial = initial;

  @override
  AppLanguage build() =>
      _initial ?? ref.watch(preferencesRepositoryProvider).language;

  /// Applies [language] immediately and persists it for the next launch.
  Future<void> select(AppLanguage language) async {
    state = language;
    await ref.read(preferencesRepositoryProvider).saveLanguage(language);
  }
}

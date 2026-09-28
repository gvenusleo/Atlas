import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart' show ThemeMode;

import 'package:atlas_flutter/features/settings/application/preferences_provider.dart';

/// Holds the client-local theme preference for the whole application.
final themeModeProvider = NotifierProvider<ThemeModeController, ThemeMode>(
  ThemeModeController.new,
);

/// Applies theme selections and delegates persistence to the settings repository.
final class ThemeModeController({ThemeMode? initial})
    extends Notifier<ThemeMode> {
  final ThemeMode? _initial = initial;

  @override
  ThemeMode build() =>
      _initial ?? ref.watch(preferencesRepositoryProvider).themeMode;

  /// Applies [mode] immediately and persists it for the next launch.
  Future<void> select(ThemeMode mode) async {
    state = mode;
    await ref.read(preferencesRepositoryProvider).saveThemeMode(mode);
  }
}

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'app/atlas_app.dart';
import 'app/locale_mode.dart';
import 'app/platform_window.dart';
import 'app/runtime_environment.dart';
import 'app/shell_environment.dart';
import 'app/theme_mode.dart';

/// Whether this build targets a phone or tablet.
///
/// Mobile clients cannot host the local Atlas runtime (configuration,
/// provider keys, and the working tree live on the computer), so they start
/// from the remote connection screen instead.
bool get isMobileClient =>
    defaultTargetPlatform == TargetPlatform.android ||
    defaultTargetPlatform == TargetPlatform.iOS;

/// Starts the application.
///
/// Desktop: composes the local runtime (kept alive until a connection is
/// activated) and starts the workspace. Mobile: no local runtime; the remote
/// connection screen is the entry point.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // The theme preference is read before the first frame so the window opens in
  // the stored appearance instead of flashing the platform default.
  final preferences = await openThemePreferences();
  final themeMode = loadThemeMode(preferences);
  final language = loadAppLanguage(preferences);
  if (!isMobileClient) {
    await initializePlatformWindow(
      initialBrightness: _effectiveBrightness(themeMode),
    );
    // Atlas ships without the App Sandbox, so the macOS file dialogs carry no
    // file-access entitlements; without this the picker refuses to open.
    await FilePicker.skipEntitlementsChecks();
  }
  final shellEnvironment = isMobileClient
      ? null
      : await resolveShellEnvironment();
  if (shellEnvironment?.failure case final failure?) {
    debugPrint(
      'Atlas shell environment: ${failure.name}; using inherited environment.',
    );
  }
  final environment = shellEnvironment?.environment;
  final bootstrap = isMobileClient
      ? null
      : await bootstrapRuntime(environment: environment);
  final controller = createRuntimeEnvironmentController(
    local: bootstrap?.environment,
    environment: environment,
  );
  runApp(
    ProviderScope(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(() => controller),
        themeModeProvider.overrideWith(
          () =>
              ThemeModeController(initial: themeMode, preferences: preferences),
        ),
        languageProvider.overrideWith(
          () => LanguageController(initial: language, preferences: preferences),
        ),
        if (!isMobileClient)
          runtimeStartupErrorProvider.overrideWithValue(bootstrap!.error),
      ],
      child: const AtlasApp(),
    ),
  );
}

/// Resolves the appearance the window and the first frame should use.
Brightness _effectiveBrightness(ThemeMode mode) {
  final platform =
      WidgetsBinding.instance.platformDispatcher.platformBrightness;
  return switch (mode) {
    ThemeMode.light => Brightness.light,
    ThemeMode.dark => Brightness.dark,
    ThemeMode.system => platform,
  };
}

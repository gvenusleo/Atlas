import 'dart:async';
import 'dart:ui' show AppExitResponse;

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../features/workspace/application/terminal_registry.dart';
import '../l10n/app_localizations.dart';
import '../shared/theme/atlas_theme.dart';
import 'app_router.dart';
import 'locale_mode.dart';
import 'platform_window.dart';
import 'runtime_environment.dart';
import 'theme_mode.dart';

/// Root application for the Atlas desktop and mobile clients.
class const AtlasApp({super.key}) extends ConsumerStatefulWidget {
  @override
  ConsumerState<AtlasApp> createState() => _AtlasAppState();
}

class _AtlasAppState extends ConsumerState<AtlasApp>
    with WidgetsBindingObserver {
  RuntimeEnvironment? _runtimeEnvironment;
  Brightness? _syncedBrightness;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _runtimeEnvironment = ref.read(runtimeEnvironmentProvider).environment;
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    unawaited(_runtimeEnvironment?.close());
    super.dispose();
  }

  @override
  Future<AppExitResponse> didRequestAppExit() async {
    // The framework asks before the embedder terminates, which is the last
    // point at which the shells can still be killed: pty2 releases each PTY
    // from a native finalizer during isolate shutdown, and closing a master
    // whose shell is still alive blocks the main thread.
    ref.read(terminalSessionRegistryProvider).closeAll();
    return AppExitResponse.exit;
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp.router(
      title: 'Atlas',
      debugShowCheckedModeBanner: false,
      theme: buildAtlasTheme(AtlasPalette.standard, Brightness.light),
      darkTheme: buildAtlasTheme(AtlasPalette.standard, Brightness.dark),
      themeMode: ref.watch(themeModeProvider),
      locale: localeForLanguage(ref.watch(languageProvider)),
      localizationsDelegates: [
        AppLocalizations.delegate,
        ...GlobalMaterialLocalizations.delegates,
      ],
      supportedLocales: atlasSupportedLocales,
      localeResolutionCallback: resolveAtlasLocale,
      routerConfig: ref.watch(appRouterProvider),
      builder: (context, child) {
        // The window chrome follows the resolved appearance, not the platform:
        // a forced light or dark mode has to repaint the native background.
        final brightness = Theme.of(context).brightness;
        if (brightness != _syncedBrightness) {
          _syncedBrightness = brightness;
          unawaited(syncPlatformWindowBackground(brightness));
        }
        return AnnotatedRegion<SystemUiOverlayStyle>(
          value: brightness == Brightness.dark
              ? SystemUiOverlayStyle.light
              : SystemUiOverlayStyle.dark,
          child: child ?? const SizedBox.shrink(),
        );
      },
    );
  }
}

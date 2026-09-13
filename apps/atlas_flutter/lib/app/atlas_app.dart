import 'dart:async';
import 'dart:ui' show AppExitResponse;

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../features/workspace/application/terminal_registry.dart';
import '../shared/theme/atlas_theme.dart';
import 'app_router.dart';
import 'platform_window.dart';
import 'runtime_environment.dart';

/// Root application for the Atlas desktop and mobile clients.
class AtlasApp extends ConsumerStatefulWidget {
  const AtlasApp({super.key});

  @override
  ConsumerState<AtlasApp> createState() => _AtlasAppState();
}

class _AtlasAppState extends ConsumerState<AtlasApp>
    with WidgetsBindingObserver {
  RuntimeEnvironment? _runtimeEnvironment;

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
  void didChangePlatformBrightness() {
    final brightness =
        WidgetsBinding.instance.platformDispatcher.platformBrightness;
    unawaited(syncPlatformWindowBackground(brightness));
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
      theme: buildAtlasTheme(Brightness.light),
      darkTheme: buildAtlasTheme(Brightness.dark),
      themeMode: ThemeMode.system,
      routerConfig: ref.watch(appRouterProvider),
      builder: (context, child) {
        final isDark = Theme.of(context).brightness == Brightness.dark;
        return AnnotatedRegion<SystemUiOverlayStyle>(
          value: isDark
              ? SystemUiOverlayStyle.light
              : SystemUiOverlayStyle.dark,
          child: child ?? const SizedBox.shrink(),
        );
      },
    );
  }
}

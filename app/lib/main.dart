import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:window_manager/window_manager.dart';

import 'ui/atlas_theme.dart';
import 'ui/workspace_shell.dart';

/// Whether the host is a desktop platform managed by window_manager.
bool get _isDesktop =>
    Platform.isMacOS || Platform.isWindows || Platform.isLinux;

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  if (_isDesktop) {
    await windowManager.ensureInitialized();
    final brightness =
        WidgetsBinding.instance.platformDispatcher.platformBrightness;
    final windowOptions = WindowOptions(
      size: const Size(1200, 760),
      minimumSize: const Size(400, 600),
      center: true,
      backgroundColor: AtlasColors.forBrightness(brightness).canvas,
      // Only macOS integrates the toolbar; other desktops keep the native
      // title bar until custom caption controls exist.
      titleBarStyle: Platform.isMacOS
          ? TitleBarStyle.hidden
          : TitleBarStyle.normal,
      windowButtonVisibility: true,
    );
    windowManager.waitUntilReadyToShow(windowOptions, () async {
      await windowManager.show();
      await windowManager.focus();
    });
  }
  runApp(const AtlasApp());
}

/// Root application for the Atlas desktop and mobile clients.
class AtlasApp extends StatefulWidget {
  const AtlasApp({super.key});

  @override
  State<AtlasApp> createState() => _AtlasAppState();
}

class _AtlasAppState extends State<AtlasApp> with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangePlatformBrightness() {
    if (!_isDesktop) {
      return;
    }
    final brightness =
        WidgetsBinding.instance.platformDispatcher.platformBrightness;
    unawaited(
      windowManager.setBackgroundColor(
        AtlasColors.forBrightness(brightness).canvas,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Atlas',
      debugShowCheckedModeBanner: false,
      theme: buildAtlasTheme(Brightness.light),
      darkTheme: buildAtlasTheme(Brightness.dark),
      themeMode: ThemeMode.system,
      home: Builder(
        builder: (context) {
          final isDark = Theme.of(context).brightness == Brightness.dark;
          return AnnotatedRegion<SystemUiOverlayStyle>(
            value: isDark
                ? SystemUiOverlayStyle.light
                : SystemUiOverlayStyle.dark,
            child: const WorkspaceShell(),
          );
        },
      ),
    );
  }
}

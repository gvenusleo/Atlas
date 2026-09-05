import 'dart:io';

import 'package:material_ui/material_ui.dart';
import 'package:window_manager/window_manager.dart';

import '../shared/theme/atlas_theme.dart';

/// Whether the host uses a desktop window managed by window_manager.
bool get usesManagedDesktopWindow =>
    Platform.isMacOS || Platform.isWindows || Platform.isLinux;

/// Initializes the native desktop window before the Flutter app starts.
Future<void> initializePlatformWindow() async {
  if (!usesManagedDesktopWindow) {
    return;
  }

  await windowManager.ensureInitialized();
  final brightness =
      WidgetsBinding.instance.platformDispatcher.platformBrightness;
  final windowOptions = WindowOptions(
    size: const Size(1200, 760),
    minimumSize: const Size(400, 600),
    center: true,
    backgroundColor: AtlasColors.forBrightness(brightness).canvas,
    // All desktop platforms integrate the toolbar: the custom caption
    // controls are painted by AtlasWindowControls on Windows and Linux.
    titleBarStyle: TitleBarStyle.hidden,
    windowButtonVisibility: true,
  );
  await windowManager.waitUntilReadyToShow(windowOptions, () async {
    // Keep the native shell at the desktop baseline even when the host restores
    // a stale miniature frame from a previous run.
    await windowManager.setSize(const Size(1200, 760));
    await windowManager.setMinimumSize(const Size(400, 600));
    await windowManager.show();
    await windowManager.focus();
  });
}

/// Synchronizes the native window background with the active Atlas theme.
Future<void> syncPlatformWindowBackground(Brightness brightness) async {
  if (!usesManagedDesktopWindow) {
    return;
  }
  await windowManager.setBackgroundColor(
    AtlasColors.forBrightness(brightness).canvas,
  );
}

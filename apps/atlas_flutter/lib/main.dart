import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'app/atlas_app.dart';
import 'app/platform_window.dart';
import 'app/runtime_environment.dart';

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
  if (!isMobileClient) {
    await initializePlatformWindow();
  }
  final bootstrap = isMobileClient ? null : await bootstrapRuntime();
  final controller = isMobileClient
      ? RuntimeEnvironmentController()
      : RuntimeEnvironmentController(local: bootstrap!.environment);
  runApp(
    ProviderScope(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(() => controller),
        if (!isMobileClient)
          runtimeStartupErrorProvider.overrideWithValue(bootstrap!.error),
      ],
      child: const AtlasApp(),
    ),
  );
}

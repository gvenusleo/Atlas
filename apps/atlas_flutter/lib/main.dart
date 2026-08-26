import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app/atlas_app.dart';
import 'app/platform_window.dart';
import 'app/runtime_environment.dart';

/// Starts the desktop application with its locally composed runtime.
///
/// ACP server connections are managed from the in-app settings dialog; the
/// local runtime stays active until the user activates a connection.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initializePlatformWindow();
  final bootstrap = await bootstrapRuntime();
  final controller = RuntimeEnvironmentController(local: bootstrap.environment);
  runApp(
    ProviderScope(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(() => controller),
        runtimeStartupErrorProvider.overrideWithValue(bootstrap.error),
      ],
      child: const AtlasApp(),
    ),
  );
}

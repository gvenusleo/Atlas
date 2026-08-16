import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app/atlas_app.dart';
import 'app/platform_window.dart';
import 'app/runtime_environment.dart';

/// Starts the desktop application with its locally composed runtime.
Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initializePlatformWindow();
  final bootstrap = bootstrapRuntime();
  runApp(
    ProviderScope(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(bootstrap.environment),
        runtimeStartupErrorProvider.overrideWithValue(bootstrap.error),
      ],
      child: const AtlasApp(),
    ),
  );
}

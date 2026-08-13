import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app/atlas_app.dart';
import 'app/platform_window.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initializePlatformWindow();
  runApp(const ProviderScope(child: AtlasApp()));
}

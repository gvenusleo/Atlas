import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/features/connections/application/runtime_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/workspace_shell.dart';

/// Entry page for the Atlas workspace route.
class const WorkspacePage({super.key}) extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) => WorkspaceShell(
    environment: ref.watch(runtimeEnvironmentProvider).environment,
    startupError: ref.watch(runtimeStartupErrorProvider),
  );
}

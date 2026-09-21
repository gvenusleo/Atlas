import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../../remote_connection/application/runtime_controller.dart';
import 'workspace_shell.dart';

/// Entry page for the Atlas workspace route.
class const WorkspacePage({super.key}) extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) => WorkspaceShell(
    environment: ref.watch(runtimeEnvironmentProvider).environment,
    startupError: ref.watch(runtimeStartupErrorProvider),
  );
}

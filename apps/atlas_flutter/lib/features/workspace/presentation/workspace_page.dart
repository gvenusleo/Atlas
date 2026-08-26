import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';

import '../../../app/runtime_environment.dart';
import 'workspace_shell.dart';

/// Entry page for the Atlas workspace route.
class WorkspacePage extends ConsumerWidget {
  const WorkspacePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) => WorkspaceShell(
    environment: ref.watch(runtimeEnvironmentProvider).environment,
    startupError: ref.watch(runtimeStartupErrorProvider),
  );
}

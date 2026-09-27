import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/terminal_registry.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/terminal_panel.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('terminal panel mounts and starts a shell', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        child: MaterialApp(
          theme: buildAtlasTheme(AtlasPalette.standard, Brightness.dark),
          home: const TerminalPanel(workingDirectory: '/tmp'),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.byType(TerminalPanel), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('terminal panel tracks its shell until it is unmounted', (
    tester,
  ) async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    final registry = container.read(terminalSessionRegistryProvider);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(
          theme: buildAtlasTheme(AtlasPalette.standard, Brightness.dark),
          home: const TerminalPanel(workingDirectory: '/tmp'),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    // The exit hook can only release PTYs the panel registered.
    expect(registry.length, 1);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(
          theme: buildAtlasTheme(AtlasPalette.standard, Brightness.dark),
          home: const SizedBox.shrink(),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(registry.isEmpty, isTrue);
  });

  testWidgets('TerminalHost keeps the same panel when switching sessions', (
    tester,
  ) async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _EmptyProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(runtime: runtime, models: [model]),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(
          theme: buildAtlasTheme(AtlasPalette.standard, Brightness.dark),
          home: Scaffold(
            body: Consumer(
              builder: (context, ref, _) {
                final workspace = ref.watch(workspaceProvider);
                return TerminalHost(
                  sessionKey: workspace.activeKey,
                  workingDirectory: '/tmp',
                );
              },
            ),
          ),
        ),
      ),
    );
    await tester.pump();
    Finder visiblePanel() => find.byType(TerminalPanel).hitTestable();
    final firstState = tester.state(visiblePanel());

    final controller = container.read(workspaceProvider.notifier);
    await controller.send('first session');
    final firstId = container.read(workspaceProvider).sessionId!;
    controller.newSession();
    await tester.pump();
    expect(tester.state(visiblePanel()), isNot(same(firstState)));
    await controller.resume(firstId);
    await tester.pump();

    expect(tester.state(visiblePanel()), same(firstState));
  });
}

class _FixedWorkingDirectory(final String path)
    extends WorkspaceWorkingDirectory {
  @override
  String build() => path;
}

final class _EmptyProvider(final ModelRef model) implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('ok')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

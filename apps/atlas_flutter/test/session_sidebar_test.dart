import 'dart:async';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/sessions_panel.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('long session history builds only visible rows', (tester) async {
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(300, 500);
    addTearDown(tester.view.reset);
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('lazy')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _FakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    addTearDown(store.close);
    for (var i = 0; i < 100; i++) {
      final session = await runtime.createSession(workingDirectory: '/tmp');
      await runtime.renameSession(session.id, 'Session $i');
    }
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(runtime: runtime, models: [model]),
          ),
        ),
      ],
    );
    addTearDown(container.dispose);
    await container.read(workspaceProvider.notifier).refreshSessions();
    final lastTitle = container.read(workspaceProvider).sessions.last.title;
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(
          theme: buildAtlasTheme(AtlasPalette.standard, Brightness.light),
          home: const Scaffold(body: SessionsPanel()),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text(lastTitle), findsNothing);
    await tester.scrollUntilVisible(find.text(lastTitle), 300);
    expect(find.text(lastTitle), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'session tile shows a running indicator while a turn is in flight',
    (tester) async {
      debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
      try {
        tester.view.devicePixelRatio = 1;
        tester.view.physicalSize = const Size(1200, 760);
        addTearDown(tester.view.reset);

        final model = ModelDescriptor(
          ref: ModelRef(
            providerId: ProviderId('test'),
            modelId: ModelId('streaming'),
          ),
          name: 'Streaming test model',
          reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
        );
        final store = DriftSessionStore.inMemory();
        final provider = _BlockingProvider(model.ref);
        final runtime = AgentRuntime(
          store: store,
          provider: provider,
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
              theme: buildAtlasTheme(AtlasPalette.standard, Brightness.light),
              home: const Scaffold(body: SessionsPanel()),
            ),
          ),
        );
        await tester.pump();

        final controller = container.read(workspaceProvider.notifier);
        final turn = controller.send('first session');
        await provider.firstStarted.future;
        await tester.pump();
        final sessionId = container.read(workspaceProvider).sessionId!;
        expect(
          find.byKey(ValueKey('session-running-${sessionId.value}')),
          findsOneWidget,
        );
        expect(
          find.byKey(ValueKey('session-completed-${sessionId.value}')),
          findsNothing,
        );

        controller.newSession();
        await tester.pump();
        expect(
          find.byKey(ValueKey('session-running-${sessionId.value}')),
          findsOneWidget,
        );

        provider.releaseFirst.complete();
        await turn;
        await tester.pump();
        expect(
          find.byKey(ValueKey('session-running-${sessionId.value}')),
          findsNothing,
        );
        expect(
          find.byKey(ValueKey('session-completed-${sessionId.value}')),
          findsOneWidget,
        );

        await tester.tap(find.text('first session'));
        await tester.pump();
        expect(
          find.byKey(ValueKey('session-completed-${sessionId.value}')),
          findsNothing,
        );
      } finally {
        debugDefaultTargetPlatformOverride = null;
      }
    },
  );
}

/// Working directory fixed for tests.
final class _FixedWorkingDirectory(final String path)
    extends WorkspaceWorkingDirectory {
  @override
  String build() => path;
}

final class _FakeProvider(final ModelRef model) implements ModelProvider {
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

final class _BlockingProvider(final ModelRef model) implements ModelProvider {
  final firstStarted = Completer<void>();
  final releaseFirst = Completer<void>();

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    if (!firstStarted.isCompleted) {
      firstStarted.complete();
      await releaseFirst.future;
    }
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('ok')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

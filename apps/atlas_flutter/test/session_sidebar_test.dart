import 'dart:async';

import 'package:atlas_flutter/app/atlas_app.dart';
import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/sessions_panel.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  group('groupSessionsByTime', () {
    test('buckets sessions by recency in fixed order', () {
      final now = DateTime.now();
      final today = DateTime(now.year, now.month, now.day);
      final sessions = [
        SessionSummary(
          id: SessionId('old'),
          title: 'Old',
          workingDirectory: '/x',
          updatedAt: today.subtract(const Duration(days: 40)).toUtc(),
        ),
        SessionSummary(
          id: SessionId('month'),
          title: 'Month',
          workingDirectory: '/x',
          updatedAt: today.subtract(const Duration(days: 15)).toUtc(),
        ),
        SessionSummary(
          id: SessionId('week'),
          title: 'Week',
          workingDirectory: '/y',
          updatedAt: today.subtract(const Duration(days: 3)).toUtc(),
        ),
        SessionSummary(
          id: SessionId('yesterday'),
          title: 'Yesterday',
          workingDirectory: '/z',
          updatedAt: today.subtract(const Duration(days: 1)).toUtc(),
        ),
        SessionSummary(
          id: SessionId('today'),
          title: 'Today',
          workingDirectory: '/w',
          updatedAt: today.add(const Duration(hours: 1)).toUtc(),
        ),
      ];

      final groups = groupSessionsByTime(sessions);

      expect(groups.map((group) => group.label), [
        'Today',
        'Yesterday',
        'This Week',
        'This Month',
        'Earlier',
      ]);
      expect(groups.first.sessions.single.id.value, 'today');
      expect(groups.last.sessions.single.id.value, 'old');
    });
  });

  testWidgets('session sidebar groups sessions by recency', (tester) async {
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
      final runtime = AgentRuntime(
        store: store,
        provider: _FakeProvider(model.ref),
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

      final controller = container.read(workspaceProvider.notifier);
      await controller.send('first session');
      controller.newSession(workingDirectory: '/tmp2');
      await controller.send('second session');
      await controller.refreshSessions();

      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: container,
          child: const AtlasApp(),
        ),
      );
      // The shell refreshes sessions after the first frame; pump past the
      // loading spinner instead of pumpAndSettle, whose animation never ends.
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 50));
      await tester.pump(const Duration(milliseconds: 50));

      final leftPanel = find.byKey(const ValueKey('atlas-left-panel'));
      // Both sessions were created today, so they share one group header;
      // each directory appears once as a session row subtitle.
      expect(
        find.descendant(of: leftPanel, matching: find.text('Today')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: leftPanel, matching: find.text('tmp2')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: leftPanel, matching: find.text('tmp')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: leftPanel, matching: find.text('first session')),
        findsOneWidget,
      );
      expect(
        find.descendant(of: leftPanel, matching: find.text('second session')),
        findsOneWidget,
      );

      // Headers participate in Tab traversal and support Enter/Space.
      final header = find.ancestor(
        of: find.byKey(const ValueKey('session-group-Today')),
        matching: find.byType(InkWell),
      );
      await _tabTo(tester, header);
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pump();
      expect(
        find.descendant(of: leftPanel, matching: find.text('first session')),
        findsNothing,
      );
      expect(
        find.descendant(of: leftPanel, matching: find.text('second session')),
        findsNothing,
      );

      await tester.sendKeyEvent(LogicalKeyboardKey.space);
      await tester.pump();
      expect(
        find.descendant(of: leftPanel, matching: find.text('first session')),
        findsOneWidget,
      );
      final firstSession = container
          .read(workspaceProvider)
          .sessions
          .firstWhere((session) => session.title == 'first session')
          .id;
      await _tabTo(
        tester,
        find.ancestor(
          of: find.descendant(
            of: leftPanel,
            matching: find.text('first session'),
          ),
          matching: find.byType(InkWell),
        ),
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 100));
      expect(container.read(workspaceProvider).sessionId, firstSession);
      // Resuming focuses the composer; Tab can return to the session action.
      await _tabTo(
        tester,
        find.ancestor(
          of: find.descendant(
            of: leftPanel,
            matching: find.text('first session'),
          ),
          matching: find.byType(InkWell),
        ),
      );
      await tester.sendKeyDownEvent(LogicalKeyboardKey.shiftLeft);
      await tester.sendKeyEvent(LogicalKeyboardKey.f10);
      await tester.sendKeyUpEvent(LogicalKeyboardKey.shiftLeft);
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.text('Rename'), findsOneWidget);
      expect(find.text('Delete'), findsOneWidget);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });

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

  testWidgets('new task menu offers here and folder actions', (tester) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.macOS;
    try {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = const Size(1200, 760);
      addTearDown(tester.view.reset);
      var picked = false;

      final model = ModelDescriptor(
        ref: ModelRef(
          providerId: ProviderId('test'),
          modelId: ModelId('streaming'),
        ),
        name: 'Streaming test model',
        reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
      );
      final store = DriftSessionStore.inMemory();
      final runtime = AgentRuntime(
        store: store,
        provider: _FakeProvider(model.ref),
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
          directoryPickerProvider.overrideWithValue(() async {
            picked = true;
            return null;
          }),
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
      await tester.pumpAndSettle();

      final button = find.byKey(const ValueKey('atlas-new-session-button'));
      expect(button, findsOneWidget);
      await _tabTo(
        tester,
        find.descendant(of: button, matching: find.byType(InkWell)),
      );
      await tester.sendKeyEvent(LogicalKeyboardKey.enter);
      await tester.pumpAndSettle();
      expect(find.text('New session here'), findsOneWidget);
      expect(find.text('New session in folder...'), findsOneWidget);

      await tester.tap(find.text('New session in folder...'));
      await tester.pumpAndSettle();
      expect(picked, isTrue);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  });

  testWidgets('session tile right-click opens rename and delete menu', (
    tester,
  ) async {
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
      final runtime = AgentRuntime(
        store: store,
        provider: _FakeProvider(model.ref),
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

      final controller = container.read(workspaceProvider.notifier);
      await controller.send('first session');
      await controller.refreshSessions();

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

      // Right-click the session tile to open the context menu.
      await tester.tap(
        find.text('first session'),
        buttons: kSecondaryMouseButton,
      );
      await tester.pumpAndSettle();

      expect(find.text('Rename'), findsOneWidget);
      expect(find.text('Delete'), findsOneWidget);
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
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

Future<void> _tabTo(WidgetTester tester, Finder target) async {
  for (var i = 0; i < 50; i++) {
    final element = tester.element(target);
    var focused = false;
    FocusManager.instance.primaryFocus?.context?.visitAncestorElements((
      ancestor,
    ) {
      if (identical(ancestor, element)) focused = true;
      return !focused;
    });
    if (focused) return;
    await tester.sendKeyEvent(LogicalKeyboardKey.tab);
    await tester.pump();
  }
  fail('The sidebar control could not be reached with Tab.');
}

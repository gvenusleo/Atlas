import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/conversation_input.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';

void main() {
  testWidgets('model menu highlight follows the mouse', (tester) async {
    await _pumpComposer(tester);

    // Open the model picker.
    await tester.tap(find.text('Model A'));
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsOneWidget);

    // The model menu keeps its fixed width instead of stretching to the edge.
    final menu = find.byWidgetPredicate(
      (widget) => widget is SizedBox && widget.width == 280,
    );
    expect(menu, findsOneWidget);
    expect(tester.getSize(menu).width, 280);

    // The gliding highlight starts on the active model (row 0).
    AnimatedPositioned highlight() =>
        tester.widget<AnimatedPositioned>(find.byType(AnimatedPositioned));
    expect(highlight().top, 0);

    // Hover the second row with a real mouse pointer.
    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await gesture.addPointer(location: Offset.zero);
    addTearDown(gesture.removePointer);
    await tester.pump();
    await gesture.moveTo(tester.getCenter(find.text('Model B')));
    await tester.pump();

    expect(highlight().top, 30);
  });

  testWidgets('clicking outside the model menu closes it', (tester) async {
    await _pumpComposer(tester);

    // Open the model picker.
    await tester.tap(find.text('Model A'));
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsOneWidget);

    // Click the input field, which sits outside the floating menu.
    await tester.tap(find.byKey(const ValueKey('atlas-prompt-input')));
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsNothing);
  });

  testWidgets('slash suggestions highlight follows the mouse', (tester) async {
    await _pumpComposer(tester);

    // Type a slash to open the command picker.
    await tester.enterText(
      find.byKey(const ValueKey('atlas-prompt-input')),
      '/',
    );
    await tester.pumpAndSettle();
    expect(find.text('/compact'), findsOneWidget);

    // Hover the second row with a real mouse pointer.
    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await gesture.addPointer(location: Offset.zero);
    addTearDown(gesture.removePointer);
    await tester.pump();
    await gesture.moveTo(tester.getCenter(find.text('/model')));
    await tester.pump();

    AnimatedPositioned highlight() =>
        tester.widget<AnimatedPositioned>(find.byType(AnimatedPositioned));
    expect(highlight().top, 30);
  });

  testWidgets('model picker supports keyboard navigation', (tester) async {
    await _pumpComposer(tester);

    // Open the model picker.
    await tester.tap(find.text('Model A'));
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsOneWidget);

    // Arrow down moves the highlight to the second row.
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.pump();

    AnimatedPositioned highlight() =>
        tester.widget<AnimatedPositioned>(find.byType(AnimatedPositioned));
    expect(highlight().top, 30);

    // Enter selects the highlighted model and closes the picker.
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsOneWidget); // Only the trigger button.
  });

  testWidgets('slash suggestions have no footer hint', (tester) async {
    await _pumpComposer(tester);

    await tester.enterText(
      find.byKey(const ValueKey('atlas-prompt-input')),
      '/',
    );
    await tester.pumpAndSettle();

    expect(find.text('Type to search commands'), findsNothing);
  });

  testWidgets('escape closes the model menu', (tester) async {
    await _pumpComposer(tester);

    // Open the model picker.
    await tester.tap(find.text('Model A'));
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsOneWidget);

    // Escape closes the picker without selecting a model.
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pumpAndSettle();
    expect(find.text('Model B'), findsNothing);
  });

  testWidgets('escape clears the slash suggestions', (tester) async {
    await _pumpComposer(tester);

    // Type a slash to open the command picker.
    await tester.enterText(
      find.byKey(const ValueKey('atlas-prompt-input')),
      '/',
    );
    await tester.pumpAndSettle();
    expect(find.text('/compact'), findsOneWidget);

    // Escape clears the input and closes the picker.
    await tester.sendKeyEvent(LogicalKeyboardKey.escape);
    await tester.pumpAndSettle();
    expect(find.text('/compact'), findsNothing);
  });

  testWidgets('model picker keyboard does not crash with no models', (
    tester,
  ) async {
    await _pumpComposer(tester, models: const []);

    // Open the model picker; the trigger shows the bare model id.
    await tester.tap(find.text('model-a'));
    await tester.pumpAndSettle();

    // Arrow keys must not throw with an empty model list.
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.arrowUp);
    await tester.pump();
  });

  testWidgets('send button shifts to the accent color on hover', (
    tester,
  ) async {
    await _pumpComposer(tester);
    await tester.enterText(
      find.byKey(const ValueKey('atlas-prompt-input')),
      'hello',
    );
    await tester.pump();

    AnimatedContainer surface() => tester.widget<AnimatedContainer>(
      find.descendant(
        of: find.byKey(const ValueKey('atlas-send-button')),
        matching: find.byType(AnimatedContainer),
      ),
    );
    BoxDecoration? decoration() => surface().decoration as BoxDecoration?;
    expect(decoration()?.color, AtlasColors.dark.textPrimary);

    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await gesture.addPointer(location: Offset.zero);
    addTearDown(gesture.removePointer);
    await tester.pump();
    await gesture.moveTo(
      tester.getCenter(find.byKey(const ValueKey('atlas-send-button'))),
    );
    await tester.pump(const Duration(milliseconds: 200));

    expect(decoration()?.color, AtlasColors.dark.accent);
  });
}

/// Pumps the composer anchored at the bottom of the screen, as in the app.
Future<void> _pumpComposer(
  WidgetTester tester, {
  List<ModelDescriptor>? models,
}) async {
  final modelA = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('model-a')),
    name: 'Model A',
    reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
  );
  final modelB = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('model-b')),
    name: 'Model B',
    reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
  );
  final allModels = models ?? [modelA, modelB];
  final store = DriftSessionStore.inMemory();
  final runtime = AgentRuntime(
    store: store,
    provider: _FakeProvider(modelA.ref),
    tools: LocalToolRegistry(const []),
    ids: SecureIdGenerator(),
    defaultModel: modelA.ref,
  );
  addTearDown(store.close);

  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: allModels,
            skills: _EmptySkillCatalog(),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => _FixedWorkingDirectory('/tmp'),
        ),
      ],
      child: MaterialApp(
        theme: buildAtlasTheme(Brightness.dark),
        home: Scaffold(
          body: Align(
            alignment: Alignment.bottomCenter,
            child: ConversationInput(),
          ),
        ),
      ),
    ),
  );
  await tester.pumpAndSettle();
}

/// Working directory fixed for tests.
final class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

final class _FakeProvider implements ModelProvider {
  _FakeProvider(this.model);

  final ModelRef model;

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

final class _EmptySkillCatalog implements SkillCatalog {
  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}

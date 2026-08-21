import 'dart:convert';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_state.dart';
import 'package:atlas_flutter/features/workspace/data/image_attachment.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/conversation_input.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/conversation_view.dart';
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

  testWidgets('enter sends the prompt when the IME is idle', (tester) async {
    await _pumpComposer(tester);
    await tester.enterText(
      find.byKey(const ValueKey('atlas-prompt-input')),
      'hello',
    );
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();

    final user = _workspaceOf(
      tester,
    ).messages.where((message) => message.kind == WorkspaceMessageKind.user);
    expect(user, hasLength(1));
    expect(user.first.text, 'hello');
  });

  testWidgets('enter does not send while the IME is composing', (tester) async {
    await _pumpComposer(tester);
    await tester.tap(find.byKey(const ValueKey('atlas-prompt-input')));
    await tester.pump();
    final field = tester.widget<TextField>(
      find.byKey(const ValueKey('atlas-prompt-input')),
    );
    field.controller!.value = const TextEditingValue(
      text: 'nihao',
      selection: TextSelection.collapsed(offset: 5),
      composing: TextRange(start: 0, end: 5),
    );
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();

    expect(
      _workspaceOf(
        tester,
      ).messages.where((message) => message.kind == WorkspaceMessageKind.user),
      isEmpty,
    );
    expect(field.controller!.text, 'nihao');
  });

  testWidgets('enter after IME commit does not send in the same frame', (
    tester,
  ) async {
    await _pumpComposer(tester);
    await tester.tap(find.byKey(const ValueKey('atlas-prompt-input')));
    await tester.pump();
    final field = tester.widget<TextField>(
      find.byKey(const ValueKey('atlas-prompt-input')),
    );
    field.controller!.value = const TextEditingValue(
      text: '你好',
      selection: TextSelection.collapsed(offset: 2),
      composing: TextRange(start: 0, end: 2),
    );
    await tester.pump();

    // compositionend clears the composing range, then Enter arrives.
    field.controller!.value = const TextEditingValue(
      text: '你好',
      selection: TextSelection.collapsed(offset: 2),
    );
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();

    expect(
      _workspaceOf(
        tester,
      ).messages.where((message) => message.kind == WorkspaceMessageKind.user),
      isEmpty,
    );

    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();
    expect(_workspaceOf(tester).messages.first.text, '你好');
  });

  test('compactTokenCount always uses the k suffix', () {
    expect(compactTokenCount(0), '0');
    expect(compactTokenCount(128), '128');
    expect(compactTokenCount(128000), '128k');
    expect(compactTokenCount(512000), '512k');
    expect(compactTokenCount(1024000), '1024k');
    expect(compactTokenCount(1500), '1.5k');
  });

  test('contextUsageLabel includes percent and compact used/window', () {
    expect(contextUsageLabel(10000, 100000), '10% · 10k/100k');
    expect(contextUsageLabel(128000, 512000), '25% · 128k/512k');
    expect(contextUsageLabel(128, 0), '128');
  });

  testWidgets('context usage ring shows compact used/window on hover', (
    tester,
  ) async {
    await _pumpComposer(
      tester,
      contextWindow: 512000,
      usage: const TokenUsage(totalTokens: 128000),
    );
    await tester.enterText(
      find.byKey(const ValueKey('atlas-prompt-input')),
      'hello',
    );
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pumpAndSettle();

    final ring = find.byKey(const ValueKey('atlas-context-usage'));
    expect(ring, findsOneWidget);
    expect(find.textContaining('tokens'), findsNothing);

    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await gesture.addPointer(location: Offset.zero);
    addTearDown(gesture.removePointer);
    await tester.pump();
    await gesture.moveTo(tester.getCenter(ring));
    await tester.pump(const Duration(milliseconds: 450));
    await tester.pumpAndSettle();
    expect(find.text('25% · 128k/512k'), findsOneWidget);
  });

  testWidgets('session pane keeps composer text after switching', (
    tester,
  ) async {
    await _pumpSessionPanes(tester);
    final prompt = find.byKey(const ValueKey('atlas-prompt-input'));
    final controller = ProviderScope.containerOf(
      tester.element(prompt.hitTestable()),
    ).read(workspaceProvider.notifier);
    await controller.send('first session');
    await tester.pumpAndSettle();
    await tester.enterText(prompt.hitTestable(), 'draft for first');
    await tester.pump();
    controller.newSession();
    await tester.pump();
    expect(
      tester.widget<TextField>(prompt.hitTestable()).controller!.text,
      isEmpty,
    );
    expect(
      tester
          .widgetList<TextField>(
            find.byKey(
              const ValueKey('atlas-prompt-input'),
              skipOffstage: false,
            ),
          )
          .map((field) => field.controller!.text),
      contains('draft for first'),
    );
  });

  testWidgets('composer model follows the focused session', (tester) async {
    await _pumpSessionPanes(tester);
    final controller = ProviderScope.containerOf(
      tester.element(find.byType(ConversationInput)),
    ).read(workspaceProvider.notifier);
    await tester.tap(find.text('Model A'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Model B').hitTestable());
    await tester.pumpAndSettle();
    expect(find.text('Model B').hitTestable(), findsOneWidget);
    expect(find.text('Model A').hitTestable(), findsNothing);

    await controller.send('first session');
    final firstId = _workspaceOf(tester).sessionId!;
    controller.newSession();
    await tester.pump();
    expect(find.text('Model B').hitTestable(), findsOneWidget);

    controller.selectModel(
      controller.models.firstWhere((model) => model.name == 'Model A'),
    );
    await tester.pump();
    expect(find.text('Model A').hitTestable(), findsOneWidget);

    await controller.resume(firstId);
    await tester.pump();
    expect(find.text('Model B').hitTestable(), findsOneWidget);
    expect(find.text('Model A').hitTestable(), findsNothing);
  });

  testWidgets('attaching an image enables send without text', (tester) async {
    await _pumpSessionPanes(
      tester,
      models: [_visionModel()],
      onPickImages: () async => [_pngImage()],
    );
    await tester.tap(find.byKey(const ValueKey('atlas-attach-image')));
    await tester.pumpAndSettle();
    expect(find.byTooltip('Remove image'), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey('atlas-send-button')));
    await tester.pumpAndSettle();
    final user = _workspaceOf(
      tester,
    ).messages.where((message) => message.kind == WorkspaceMessageKind.user);
    expect(user, hasLength(1));
    expect(user.first.text, isEmpty);
    expect(user.first.imageSources, hasLength(1));
    expect(find.byTooltip('Remove image'), findsNothing);
    expect(find.byType(Image), findsOneWidget);
  });

  testWidgets('pasting an image attaches it to the composer', (tester) async {
    await _pumpComposer(
      tester,
      models: [_visionModel()],
      onPasteImages: () async => [_pngImage()],
    );
    await tester.tap(find.byKey(const ValueKey('atlas-prompt-input')));
    await tester.pump();
    final modifier = defaultTargetPlatform == TargetPlatform.macOS
        ? LogicalKeyboardKey.meta
        : LogicalKeyboardKey.control;
    await tester.sendKeyDownEvent(modifier);
    await tester.sendKeyEvent(LogicalKeyboardKey.keyV);
    await tester.sendKeyUpEvent(modifier);
    await tester.pumpAndSettle();
    expect(find.byTooltip('Remove image'), findsOneWidget);
  });

  testWidgets('attach is disabled when the model cannot accept images', (
    tester,
  ) async {
    await _pumpComposer(tester);
    final button = tester.widget<IconButton>(
      find.byKey(const ValueKey('atlas-attach-image')),
    );
    expect(button.onPressed, isNull);
  });
}

WorkspaceState _workspaceOf(WidgetTester tester) {
  return ProviderScope.containerOf(
    tester.element(find.byType(ConversationInput)),
  ).read(workspaceProvider);
}

/// Pumps the composer anchored at the bottom of the screen, as in the app.
Future<void> _pumpComposer(
  WidgetTester tester, {
  List<ModelDescriptor>? models,
  int contextWindow = 0,
  TokenUsage usage = const TokenUsage(),
  Future<List<PendingImage>> Function()? onPickImages,
  Future<List<PendingImage>> Function()? onPasteImages,
}) async {
  await _pumpWorkspace(
    tester,
    models: models,
    contextWindow: contextWindow,
    usage: usage,
    onPickImages: onPickImages,
    onPasteImages: onPasteImages,
    child: const Align(
      alignment: Alignment.bottomCenter,
      child: ConversationInput(),
    ),
  );
}

Future<void> _pumpSessionPanes(
  WidgetTester tester, {
  List<ModelDescriptor>? models,
  Future<List<PendingImage>> Function()? onPickImages,
  Future<List<PendingImage>> Function()? onPasteImages,
}) async {
  await _pumpWorkspace(
    tester,
    models: models,
    onPickImages: onPickImages,
    onPasteImages: onPasteImages,
    child: const SessionPaneHost(),
  );
}

Future<void> _pumpWorkspace(
  WidgetTester tester, {
  required Widget child,
  List<ModelDescriptor>? models,
  int contextWindow = 0,
  TokenUsage usage = const TokenUsage(),
  Future<List<PendingImage>> Function()? onPickImages,
  Future<List<PendingImage>> Function()? onPasteImages,
}) async {
  final modelA = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('model-a')),
    name: 'Model A',
    contextWindow: contextWindow,
    reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
  );
  final modelB = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('model-b')),
    name: 'Model B',
    contextWindow: contextWindow,
    reasoningEfforts: const [ReasoningEffortOption(value: 'balanced')],
  );
  final allModels = models ?? [modelA, modelB];
  final defaultModel = allModels.firstOrNull ?? modelA;
  final store = DriftSessionStore.inMemory();
  final runtime = AgentRuntime(
    store: store,
    provider: _FakeProvider(defaultModel.ref, usage: usage),
    tools: LocalToolRegistry(const []),
    ids: SecureIdGenerator(),
    defaultModel: defaultModel.ref,
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
        if (onPickImages != null)
          imagePickerProvider.overrideWithValue(onPickImages),
        if (onPasteImages != null)
          imageClipboardProvider.overrideWithValue(onPasteImages),
      ],
      child: MaterialApp(
        theme: buildAtlasTheme(Brightness.dark),
        home: Scaffold(body: child),
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
  _FakeProvider(this.model, {this.usage = const TokenUsage()});

  final ModelRef model;
  final TokenUsage usage;

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested, contextWindow: 512000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    yield ModelCompletedEvent(
      ModelResponse(
        content: const [TextContent('ok')],
        stopReason: StopReason.endTurn,
        usage: usage,
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

ModelDescriptor _visionModel() => ModelDescriptor(
  ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
  name: 'Vision',
  inputCapabilities: const {
    ModelInputCapability.text,
    ModelInputCapability.image,
  },
);

PendingImage _pngImage() => PendingImage(
  bytes: Uint8List.fromList(
    base64Decode(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
    ),
  ),
  mimeType: 'image/png',
  name: 'dot.png',
);

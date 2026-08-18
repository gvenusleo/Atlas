import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lucide_icons_flutter/lucide_icons.dart';
import 'package:material_ui/material_ui.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/conversation_view.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';

void main() {
  testWidgets('renders a tool message with a long name', (tester) async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _ToolFakeProvider(model.ref),
      tools: LocalToolRegistry([_FakeTool()]),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
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
          theme: buildAtlasTheme(Brightness.light),
          home: const Scaffold(body: ConversationView()),
        ),
      ),
    );
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('run a tool');
    await tester.pumpAndSettle();

    final state = container.read(workspaceProvider);
    final tool = state.messages.firstWhere(
      (m) => m.kind == WorkspaceMessageKind.tool,
    );
    expect(tool.startedAt, isNotNull);
    expect(tool.isRunning, isFalse);

    expect(find.text('Fake_tool'), findsOneWidget);
    await tester.tap(find.text('Fake_tool'));
    await tester.pumpAndSettle();
    expect(find.text('file list'), findsOneWidget);
  });

  testWidgets('tool title keeps the arrow cursor on hover', (tester) async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _ToolFakeProvider(model.ref),
      tools: LocalToolRegistry([_FakeTool()]),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
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
          theme: buildAtlasTheme(Brightness.light),
          home: const Scaffold(body: ConversationView()),
        ),
      ),
    );
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('run a tool');
    await tester.pumpAndSettle();

    final gesture = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await gesture.addPointer(location: Offset.zero);
    addTearDown(gesture.removePointer);
    await tester.pump();
    await gesture.moveTo(tester.getCenter(find.text('Fake_tool')));
    await tester.pump();

    final context = tester.element(find.text('Fake_tool'));
    final data = ListTileTheme.of(context);
    expect(
      data.mouseCursor?.resolve(<WidgetState>{}),
      SystemMouseCursors.basic,
    );
  });

  testWidgets('renders a task list checkbox in assistant markdown', (
    tester,
  ) async {
    final model = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: _CheckboxFakeProvider(model.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: model.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWithValue(
          RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
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
          theme: buildAtlasTheme(Brightness.light),
          home: const Scaffold(body: ConversationView()),
        ),
      ),
    );
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('show a task list');
    await tester.pumpAndSettle();

    expect(find.byIcon(LucideIcons.squareCheckBig), findsOneWidget);
    expect(find.text('done'), findsOneWidget);
  });
}

final class _CheckboxFakeProvider implements ModelProvider {
  _CheckboxFakeProvider(this.model);

  final ModelRef model;

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    expect(request.model, model);
    yield const TextDeltaEvent('- [x] done');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('- [x] done')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class _ToolFakeProvider implements ModelProvider {
  _ToolFakeProvider(this.model);

  final ModelRef model;
  var _calls = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    expect(request.model, model);
    _calls++;
    if (_calls == 1) {
      yield ModelCompletedEvent(
        ModelResponse(
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'fake_tool',
              arguments: {'path': '/tmp'},
            ),
          ],
          stopReason: StopReason.toolUse,
        ),
      );
      return;
    }
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('done')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class _FakeTool implements Tool {
  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'fake_tool',
    description: 'A fake tool',
    inputSchema: <String, Object?>{},
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async =>
      const ToolResult(content: 'file list');
}

final class _EmptySkillCatalog implements SkillCatalog {
  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}

final class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

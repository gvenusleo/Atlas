import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/rendering.dart';
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
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
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

    expect(find.textContaining('Fake_tool'), findsOneWidget);
    expect(find.text('file list'), findsNothing);
    await tester.tap(find.textContaining('Fake_tool'));
    await tester.pumpAndSettle();
    expect(find.text('file list'), findsOneWidget);
    expect(find.textContaining('"path"'), findsNothing);
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
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
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
    await gesture.moveTo(tester.getCenter(find.textContaining('Fake_tool')));
    await tester.pump();

    expect(
      RendererBinding.instance.mouseTracker.debugDeviceActiveCursor(1),
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
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [model],
              skills: _EmptySkillCatalog(),
            ),
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

  testWidgets('read title shows a relative path and hides arguments', (
    tester,
  ) async {
    await _pumpToolConversation(
      tester,
      toolName: 'read',
      arguments: {'path': '/tmp/src/main.dart'},
      result: 'void main() {}',
    );

    expect(find.textContaining('Read'), findsOneWidget);
    expect(find.textContaining('src/main.dart'), findsOneWidget);
    expect(find.text('void main() {}'), findsNothing);
    await tester.tap(find.textContaining('src/main.dart'));
    await tester.pumpAndSettle();
    expect(find.text('void main() {}'), findsOneWidget);
    expect(find.textContaining('"path"'), findsNothing);
  });

  testWidgets('shell title shows the command and hides arguments', (
    tester,
  ) async {
    await _pumpToolConversation(
      tester,
      toolName: 'shell',
      arguments: {'command': 'ls -la'},
      result: 'AGENTS.md',
    );

    expect(find.textContaining('Shell'), findsOneWidget);
    expect(find.textContaining('ls -la'), findsOneWidget);
    await tester.tap(find.textContaining('ls -la'));
    await tester.pumpAndSettle();
    expect(find.text('AGENTS.md'), findsOneWidget);
    expect(find.textContaining('"command"'), findsNothing);
  });

  testWidgets('plan title shows completed counts and lists steps', (
    tester,
  ) async {
    await _pumpToolConversation(
      tester,
      toolName: 'plan',
      arguments: {
        'plan': [
          {'step': 'Inspect files', 'status': 'completed'},
          {'step': 'Write tests', 'status': 'in_progress'},
          {'step': 'Ship it', 'status': 'pending'},
        ],
      },
      result: 'Plan updated',
    );

    expect(find.textContaining('Plan'), findsWidgets);
    expect(find.textContaining('1/3 completed'), findsOneWidget);
    expect(find.text('Plan updated'), findsNothing);
    await tester.tap(find.textContaining('1/3 completed'));
    await tester.pumpAndSettle();
    expect(find.text('Inspect files'), findsOneWidget);
    expect(find.text('Write tests'), findsOneWidget);
    expect(find.text('Ship it'), findsOneWidget);
    expect(find.text('Plan updated'), findsNothing);
  });
}

Future<void> _pumpToolConversation(
  WidgetTester tester, {
  required String toolName,
  required JsonObject arguments,
  required String result,
}) async {
  final model = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
  );
  final store = DriftSessionStore.inMemory();
  final runtime = AgentRuntime(
    store: store,
    provider: _NamedToolFakeProvider(model.ref, toolName, arguments),
    tools: LocalToolRegistry([_NamedFakeTool(toolName, result)]),
    ids: SecureIdGenerator(),
    defaultModel: model.ref,
  );
  final container = ProviderContainer(
    overrides: [
      runtimeEnvironmentProvider.overrideWith(
        () => RuntimeEnvironmentController(
          local: RuntimeEnvironment(
            runtime: runtime,
            models: [model],
            skills: _EmptySkillCatalog(),
          ),
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
  await container.read(workspaceProvider.notifier).send('run a tool');
  await tester.pumpAndSettle();
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

final class _NamedToolFakeProvider implements ModelProvider {
  _NamedToolFakeProvider(this.model, this.toolName, this.arguments);

  final ModelRef model;
  final String toolName;
  final JsonObject arguments;
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
              name: toolName,
              arguments: arguments,
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

final class _NamedFakeTool implements Tool {
  _NamedFakeTool(this.name, this.result);

  final String name;
  final String result;

  @override
  ToolDescriptor get descriptor => ToolDescriptor(
    name: name,
    description: 'A fake $name tool',
    inputSchema: const <String, Object?>{},
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async =>
      ToolResult(content: result);
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

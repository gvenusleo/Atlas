import 'dart:async';

import 'package:atlas_acp/atlas_acp.dart';
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
  testWidgets('manual shell collapse survives scrolling away and back', (
    tester,
  ) async {
    await tester.runAsync(() async {
      final model = ModelDescriptor(
        ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
      );
      final tool = _StreamingShellTool();
      final store = DriftSessionStore.inMemory();
      final runtime = AgentRuntime(
        store: store,
        provider: _NamedToolFakeProvider(model.ref, 'shell', {
          'command': 'cascade build',
        }),
        tools: LocalToolRegistry([tool]),
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
      addTearDown(() async {
        if (!tool.finish.isCompleted) tool.finish.complete();
        container.dispose();
        await store.close();
      });
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
      controller.state = controller.state.copyWith(
        workspaces: {
          ...controller.state.workspaces,
          controller.state.activeKey: controller.state.active.copyWith(
            messages: [
              for (var i = 0; i < 100; i++)
                WorkspaceMessage(
                  id: 'history-$i',
                  kind: WorkspaceMessageKind.user,
                  text: 'History row $i ${'old text ' * 20}',
                ),
            ],
          ),
        },
      );
      await tester.pump();
      final sending = controller.send('run');
      await tool.started.future;
      final live = Completer<void>();
      final listener = container.listen(workspaceProvider, (_, state) {
        if (state.messages.any((m) => m.text == 'live output') &&
            !live.isCompleted) {
          live.complete();
        }
      });
      tool.emit('live output');
      await live.future;
      listener.close();
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.text('live output'), findsOneWidget);
      await tester.tap(find.textContaining('cascade build'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.text('live output'), findsNothing);
      final scroll = tester.widget<ListView>(find.byType(ListView)).controller!;
      scroll.jumpTo(scroll.position.maxScrollExtent);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(find.textContaining('cascade build'), findsNothing);
      scroll.jumpTo(0);
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      tool.finish.complete();
      await sending;
      expect(
        find.text('live output'),
        findsNothing,
        reason: 'manual collapse should survive scrolling away and back',
      );
    });
  });

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

  for (final width in [800.0, 375.0]) {
    testWidgets('shows live shell output through ACP at width $width', (
      tester,
    ) async {
      tester.view.physicalSize = Size(width, 700);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      // Keep ACP transport setup and teardown in the real async zone.
      await tester.runAsync(() async {
        final model = ModelDescriptor(
          ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('m')),
        );
        final tool = _StreamingShellTool();
        final store = DriftSessionStore.inMemory();
        final runtime = AgentRuntime(
          store: store,
          provider: _NamedToolFakeProvider(model.ref, 'shell', {
            'command': 'build project',
          }),
          tools: LocalToolRegistry([tool]),
          ids: SecureIdGenerator(),
          defaultModel: model.ref,
        );
        final (serverDone, transport) = AcpServer(
          runtime,
          models: [model],
        ).serveMemory();
        final client = AcpClient(
          transport,
          catalog: [model],
          defaultModel: model.ref,
        );
        await client.connect();
        final container = ProviderContainer(
          overrides: [
            runtimeEnvironmentProvider.overrideWith(
              () => RuntimeEnvironmentController(
                local: RuntimeEnvironment(runtime: client, models: [model]),
              ),
            ),
            workspaceWorkingDirectoryProvider.overrideWith(
              () => _FixedWorkingDirectory('/tmp'),
            ),
          ],
        );
        addTearDown(() async {
          if (!tool.finish.isCompleted) tool.finish.complete();
          container.dispose();
          await client.close();
          await serverDone;
          await store.close();
        });
        await tester.pumpWidget(
          UncontrolledProviderScope(
            container: container,
            child: MaterialApp(
              theme: buildAtlasTheme(Brightness.light),
              home: const Scaffold(body: ConversationView()),
            ),
          ),
        );
        Future<void> emit(String text) async {
          final delivered = Completer<void>();
          final listener = container.listen(workspaceProvider, (_, state) {
            if (state.messages.any(
                  (message) =>
                      message.kind == WorkspaceMessageKind.tool &&
                      message.text == text,
                ) &&
                !delivered.isCompleted) {
              delivered.complete();
            }
          });
          tool.emit(text);
          await delivered.future;
          listener.close();
          await tester.pump();
          await tester.pump(const Duration(milliseconds: 300));
        }

        final sending = container
            .read(workspaceProvider.notifier)
            .send('run a build');
        await tool.started.future;
        await emit('Compiling first file');
        expect(find.text('Compiling first file'), findsOneWidget);
        expect(
          container.read(workspaceProvider).messages.last.isRunning,
          isTrue,
        );
        await tester.tap(find.textContaining('build project'));
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 300));
        await emit('Compiling second file');
        expect(find.text('Compiling second file'), findsNothing);
        tool.finish.complete();
        await sending.timeout(const Duration(seconds: 3));
        await tester.pumpAndSettle();
        final messages = container
            .read(workspaceProvider)
            .messages
            .where((message) => message.kind == WorkspaceMessageKind.tool);
        expect(messages, hasLength(1));
        expect(messages.single.isRunning, isFalse);
        expect(messages.single.text, 'Build complete');
        expect(find.text('Build complete'), findsNothing);
        await tester.tap(find.textContaining('build project'));
        await tester.pumpAndSettle();
        expect(find.text('Build complete'), findsOneWidget);
        expect(tester.takeException(), isNull);
      });
    });
  }

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
    expectSync(request.model, model);
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

final class _FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  _FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

final class _StreamingShellTool implements Tool {
  final started = Completer<void>();
  final finish = Completer<void>();
  late ToolContext _context;

  @override
  ToolDescriptor get descriptor => const ToolDescriptor(
    name: 'shell',
    description: 'Streaming shell fixture',
    inputSchema: {},
  );

  void emit(String text) => _context.onOutput!(
    ToolOutputSnapshot(
      content: text,
      totalBytes: text.length,
      truncated: false,
    ),
  );

  @override
  Future<ToolResult> execute(ToolContext context, JsonObject arguments) async {
    _context = context;
    started.complete();
    await finish.future;
    return const ToolResult(content: 'Build complete');
  }
}

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:atlas_flutter/app/runtime_environment.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';
import 'package:atlas_flutter/features/workspace/application/workspace_message.dart';

import 'workspace_support.dart';

void main() {
  final model = ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('slash')),
    name: 'Slash test model',
  );

  ({ProviderContainer container, FakeModeRuntime runtime}) setUpContainer() {
    final store = DriftSessionStore.inMemory();
    final runtime = FakeModeRuntime(
      AgentRuntime(
        store: store,
        provider: FakeProvider(model.ref),
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: model.ref,
      ),
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(runtime: runtime, models: [model]),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);
    return (container: container, runtime: runtime);
  }

  test(
    '/compact compacts the session instead of running a model turn',
    () async {
      final (:container, :runtime) = setUpContainer();
      final controller = container.read(workspaceProvider.notifier);
      await controller.send('hello');
      final messagesAfterTurn = container
          .read(workspaceProvider)
          .messages
          .length;

      final sent = await controller.send('/compact keep the plan');

      expect(sent, isTrue);
      expect(runtime.compactCalls, hasLength(1));
      expect(runtime.compactCalls.single.$2, 'keep the plan');
      // The command is consumed by the controller: it never reaches the model
      // and never appears as a user message.
      expect(runtime.turnModes, hasLength(1));
      final messages = container.read(workspaceProvider).messages;
      expect(messages, hasLength(messagesAfterTurn));
      expect(
        messages.where((message) => message.kind == WorkspaceMessageKind.user),
        hasLength(1),
      );
    },
  );

  test('/compact without arguments carries no instruction', () async {
    final (:container, :runtime) = setUpContainer();
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('hello');

    await controller.send('/compact');

    expect(runtime.compactCalls.single.$2, isNull);
  });

  test(
    'only the exact compact command is treated as a slash command',
    () async {
      final (:container, :runtime) = setUpContainer();
      final controller = container.read(workspaceProvider.notifier);

      // Tokens that merely start with the command name are ordinary prompts.
      for (final text in ['/compact-now', '/compaction']) {
        final before = runtime.turnModes.length;
        await controller.send(text);
        expect(runtime.compactCalls, isEmpty, reason: 'for "$text"');
        expect(runtime.turnModes, hasLength(before + 1), reason: 'for "$text"');
      }
      final messages = container.read(workspaceProvider).messages;
      expect(
        messages.where((message) => message.kind == WorkspaceMessageKind.user),
        hasLength(2),
      );
    },
  );

  test('a padded compact command still compacts', () async {
    final (:container, :runtime) = setUpContainer();
    final controller = container.read(workspaceProvider.notifier);
    await controller.send('hello');

    // send() trims the prompt first, so padding never defers the command to
    // the model, and the trailing words stay the compaction instruction.
    await controller.send('  /compact  explain the plan  ');

    expect(runtime.compactCalls.single.$2, 'explain the plan');
    expect(runtime.turnModes, hasLength(1));
  });

  test('an unknown slash command is sent to the model as text', () async {
    final (:container, :runtime) = setUpContainer();
    final controller = container.read(workspaceProvider.notifier);

    final sent = await controller.send('/unknown');

    expect(sent, isTrue);
    expect(runtime.compactCalls, isEmpty);
    expect(runtime.turnModes, hasLength(1));
    final messages = container.read(workspaceProvider).messages;
    expect(messages.first.kind, WorkspaceMessageKind.user);
    expect(messages.first.text, '/unknown');
    expect(messages.last.kind, WorkspaceMessageKind.assistant);
  });

  test('a draft session reports that there is nothing to compact', () async {
    final (:container, :runtime) = setUpContainer();
    final controller = container.read(workspaceProvider.notifier);

    final sent = await controller.send('/compact');

    expect(sent, isTrue);
    expect(runtime.compactCalls, isEmpty);
    expect(runtime.turnModes, isEmpty);
    final messages = container.read(workspaceProvider).messages;
    expect(messages.single.kind, WorkspaceMessageKind.notice);
    expect(messages.single.text, 'No session to compact.');
  });
}

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
  test('send attaches images to the user turn', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final store = DriftSessionStore.inMemory();
    final provider = RecordingProvider();
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: visionModel.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [visionModel],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    const image = ImageContent(
      source: 'data:image/png;base64,AAAA',
      mimeType: 'image/png',
    );
    final controller = container.read(workspaceProvider.notifier);
    final sent = await controller.send('describe this', images: const [image]);
    expect(sent, isTrue);

    final state = container.read(workspaceProvider);
    expect(state.hasImages, isTrue);
    expect(state.messages.first.kind, WorkspaceMessageKind.user);
    expect(state.messages.first.text, 'describe this');
    expect(state.messages.first.imageSources, [image.source]);
    expect(
      provider.contents.first.whereType<ImageContent>().single.source,
      image.source,
    );
  });
  test('send rejects images for slash commands', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: FakeProvider(visionModel.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: visionModel.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [visionModel],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    final controller = container.read(workspaceProvider.notifier);
    final sent = await controller.send(
      '/compact',
      images: const [ImageContent(source: 'data:image/png;base64,AAAA')],
    );
    expect(sent, isFalse);
    final state = container.read(workspaceProvider);
    expect(state.busy, isFalse);
    expect(state.messages.single.kind, WorkspaceMessageKind.notice);
    expect(state.messages.single.text, contains('do not support images'));
  });
  test('send rejects images when the model cannot accept them', () async {
    final textModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('text')),
      name: 'Text only',
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: FakeProvider(textModel.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: textModel.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [textModel],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    final controller = container.read(workspaceProvider.notifier);
    final sent = await controller.send(
      'look',
      images: const [ImageContent(source: 'data:image/png;base64,AAAA')],
    );
    expect(sent, isFalse);
    expect(
      container.read(workspaceProvider).messages.single.text,
      contains('does not support image input'),
    );
  });
  test('resume restores user image thumbnails', () async {
    final visionModel = ModelDescriptor(
      ref: ModelRef(providerId: ProviderId('test'), modelId: ModelId('vision')),
      name: 'Vision model',
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final store = DriftSessionStore.inMemory();
    final runtime = AgentRuntime(
      store: store,
      provider: FakeProvider(visionModel.ref),
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: visionModel.ref,
    );
    final container = ProviderContainer(
      overrides: [
        runtimeEnvironmentProvider.overrideWith(
          () => RuntimeEnvironmentController(
            local: RuntimeEnvironment(
              runtime: runtime,
              models: [visionModel],
              skills: EmptySkillCatalog(),
            ),
          ),
        ),
        workspaceWorkingDirectoryProvider.overrideWith(
          () => FixedWorkingDirectory('/tmp'),
        ),
      ],
    );
    addTearDown(store.close);
    addTearDown(container.dispose);

    const image = ImageContent(
      source: 'data:image/png;base64,AAAA',
      mimeType: 'image/png',
    );
    final session = await runtime.createSession(workingDirectory: '/tmp');
    await runtime
        .run(
          TurnRequest(
            sessionId: session.id,
            content: const [TextContent('describe this'), image],
            workingDirectory: '/tmp',
            model: visionModel.ref,
          ),
        )
        .toList();

    await container.read(workspaceProvider.notifier).resume(session.id);
    final user = container
        .read(workspaceProvider)
        .messages
        .firstWhere((message) => message.kind == WorkspaceMessageKind.user);
    expect(user.text, 'describe this');
    expect(user.imageSources, [image.source]);
  });
}

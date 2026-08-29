import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_runtime/src/agent/model_request_composer.dart'
    show maxSelectedSkillBytes;
import 'package:test/test.dart';

import 'test_fakes.dart';

void main() {
  test('omits images when the model cannot accept them', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider(
      const [
        ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
      inputCapabilities: const {ModelInputCapability.text},
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: ThrowingTools(),
      ids: TestIds(),
      defaultModel: testModel,
    );

    await runtime
        .run(
          const TurnRequest(
            content: [
              TextContent('Describe this'),
              ImageContent(source: 'data:image/png;base64,AAAA'),
            ],
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    final user = provider.requests.single.messages.firstWhere(
      (message) => message.role == ModelMessageRole.user,
    );
    expect(user.content, const [
      TextContent('Describe this'),
      TextContent(
        '[image omitted: current model does not support image input]',
      ),
    ]);
    // The persisted timeline keeps the original image content.
    final userItem = store.timeline.firstWhere(
      (item) => item is UserMessageItem,
    );
    expect(
      (userItem as UserMessageItem).content.whereType<ImageContent>(),
      hasLength(1),
    );
    expect(store.turns.single.status, TurnStatus.completed);
  });
  test('keeps images when the model supports image input', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider(
      const [
        ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
        ),
      ],
      inputCapabilities: const {
        ModelInputCapability.text,
        ModelInputCapability.image,
      },
    );
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: ThrowingTools(),
      ids: TestIds(),
      defaultModel: testModel,
    );

    await runtime
        .run(
          const TurnRequest(
            content: [
              TextContent('Describe this'),
              ImageContent(source: 'data:image/png;base64,AAAA'),
            ],
            workingDirectory: '/tmp',
          ),
        )
        .toList();

    final user = provider.requests.single.messages.firstWhere(
      (message) => message.role == ModelMessageRole.user,
    );
    expect(user.content.whereType<ImageContent>(), hasLength(1));
    expect(store.turns.single.status, TurnStatus.completed);
  });
  test('injects selected skills as leading non-persistent messages', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Done.')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
      sessionContextBuilder: contextBuilder(
        MemorySkillCatalog(const [
          Skill(
            name: 'alpha',
            description: 'Alpha skill.',
            dir: '/skills/alpha',
            path: '/skills/alpha/SKILL.md',
            content: '# Alpha\n\nFollow these steps.',
          ),
        ]),
      ),
    );

    await runtime
        .run(
          const TurnRequest(
            content: [TextContent('Use alpha')],
            workingDirectory: '/tmp',
            skills: ['alpha', 'missing', 'alpha'],
          ),
        )
        .toList();

    final messages = provider.requests.single.messages;
    expect(messages.first.role, ModelMessageRole.user);
    expect(
      textFromContent(messages.first.content),
      contains(
        '<skill>\n<name>alpha</name>\n<path>/skills/alpha/SKILL.md</path>',
      ),
    );
    expect(textFromContent(messages.first.content), contains('Follow these'));
    expect(messages.last.role, ModelMessageRole.user);
    expect(textFromContent(messages.last.content), 'Use alpha');
    expect(store.timeline, hasLength(2));
    expect(store.timeline.whereType<UserMessageItem>().single.content, const [
      TextContent('Use alpha'),
    ]);
    expect(
      store.timeline.whereType<AssistantMessageItem>().single.content,
      const [TextContent('Done.')],
    );
  });
  test('skips unknown and disabled skills when injecting', () async {
    final store = MemorySessionStore();
    final provider = ScriptedProvider([
      const ModelResponse(
        content: [TextContent('Done.')],
        stopReason: StopReason.endTurn,
      ),
    ]);
    final runtime = AgentRuntime(
      store: store,
      provider: provider,
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
      sessionContextBuilder: contextBuilder(
        MemorySkillCatalog(const [
          Skill(
            name: 'hidden',
            description: 'Hidden.',
            dir: '/skills/hidden',
            path: '/skills/hidden/SKILL.md',
            content: '# Hidden',
            disableModelInvocation: true,
          ),
        ]),
      ),
    );

    await runtime
        .run(
          const TurnRequest(
            content: [TextContent('Ask')],
            workingDirectory: '/tmp',
            skills: ['missing', 'hidden'],
          ),
        )
        .toList();

    expect(provider.requests.single.messages, hasLength(1));
    expect(
      textFromContent(provider.requests.single.messages.single.content),
      'Ask',
    );
  });
  test(
    'fails the turn when selected skill instructions are too large',
    () async {
      final store = MemorySessionStore();
      final runtime = AgentRuntime(
        store: store,
        provider: ScriptedProvider(const []),
        tools: MemoryTools(result: const ToolResult(content: 'unused')),
        ids: TestIds(),
        defaultModel: testModel,
        sessionContextBuilder: contextBuilder(
          MemorySkillCatalog([
            Skill(
              name: 'huge',
              description: 'Huge.',
              dir: '/skills/huge',
              path: '/skills/huge/SKILL.md',
              content: 'x' * (maxSelectedSkillBytes + 1),
            ),
          ]),
        ),
      );

      final events = <AgentEvent>[];
      await expectLater(() async {
        await for (final event in runtime.run(
          const TurnRequest(
            content: [TextContent('Use huge')],
            workingDirectory: '/tmp',
            skills: ['huge'],
          ),
        )) {
          events.add(event);
        }
      }(), throwsStateError);

      expect(store.turns.single.status, TurnStatus.failed);
      expect(events.last, isA<TurnFinished>());
      expect(
        (events.last as TurnFinished).outcome.failure?.code,
        'turn_failed',
      );
    },
  );
}

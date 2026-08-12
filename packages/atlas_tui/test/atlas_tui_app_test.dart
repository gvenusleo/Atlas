import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_storage/atlas_storage.dart';
import 'package:atlas_tools/atlas_tools.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  test('renders the chat surface and submits a turn', () async {
    await testNocterm('chat app', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = AgentRuntime(
        store: DriftSessionStore.inMemory(),
        provider: provider,
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: ModelRef(
          providerId: ProviderId('fake'),
          modelId: ModelId('model'),
        ),
      );
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      expect(tester.terminalState, containsText('Message Atlas'));

      await tester.enterText('hello there');
      await tester.sendEnter();
      // The focused input cursor keeps scheduling frames, so pumpAndSettle
      // would never settle; pump a bounded number of frames instead while the
      // asynchronous turn completes.
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      expect(tester.terminalState, containsText('hello there'));
      expect(provider.streamCalls, greaterThan(0));
      // The status line below the input bar reflects the default model and
      // the context usage accumulated by the turn.
      expect(tester.terminalState, containsText('  model'));
      expect(tester.terminalState, containsText('Context 0% used'));
    });
  });

  test('keeps the draft input while a turn is running', () async {
    await testNocterm('chat app busy input', (tester) async {
      final provider = _ScriptedProvider()..gate = Completer<void>();
      final runtime = AgentRuntime(
        store: DriftSessionStore.inMemory(),
        provider: provider,
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: ModelRef(
          providerId: ProviderId('fake'),
          modelId: ModelId('model'),
        ),
      );
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('pending draft');
      await tester.sendEnter();
      await tester.pump();

      // The turn is still running and the draft is not cleared.
      expect(tester.terminalState, containsText('pending draft'));
      expect(provider.streamCalls, 1);

      provider.gate!.complete();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
    });
  });

  test('esc interrupts a running turn and clears the status row', () async {
    await testNocterm('esc interrupts turn', (tester) async {
      final provider = _ScriptedProvider()..gate = Completer<void>();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('slow turn');
      await tester.sendEnter();
      await tester.pump();
      expect(provider.streamCalls, 1);
      expect(tester.terminalState, containsText('Working'));

      await tester.sendEscape();
      await tester.pump();
      provider.gate!.complete();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();

      // The turn ended through cancellation, not failure, and the notice is
      // shown in the transcript.
      expect(tester.terminalState.getText(), isNot(contains('Working')));
      expect(tester.terminalState.getText(), isNot(contains('failed')));
      expect(tester.terminalState, containsText('Turn cancelled'));
    });
  });

  test('shows the slash popup while typing a command', () async {
    await testNocterm('slash popup', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          skills: _testSkills,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      expect(tester.terminalState, containsText('/model'));
      expect(tester.terminalState, containsText('/new'));
      expect(tester.terminalState, containsText('/compact'));
      expect(tester.terminalState, containsText('/quit'));
      expect(tester.terminalState, containsText('[Skill] Review code.'));
    });
  });

  test('hides skills that cannot be triggered from the popup', () async {
    await testNocterm('slash skill filter', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          skills: _TestSkillCatalog(const [
            Skill(
              name: 'check',
              description: 'Review code.',
              dir: '/skills/check',
              path: '/skills/check/SKILL.md',
              content: '# Check\n\nReview the diff.',
            ),
            Skill(
              name: 'bad name',
              description: 'Unselectable.',
              dir: '/skills/bad',
              path: '/skills/bad/SKILL.md',
              content: '# Bad',
            ),
            Skill(
              name: 'model',
              description: 'Builtin clash.',
              dir: '/skills/model',
              path: '/skills/model/SKILL.md',
              content: '# Model',
            ),
          ]),
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      final text = tester.terminalState.getText();
      expect(text, contains('[Skill] Review code.'));
      expect(text, isNot(contains('/bad')));
      expect('/model'.allMatches(text).length, 1);
    });
  });

  test('submits a skill command with its instructions injected', () async {
    await testNocterm('slash skill', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider, skills: _testSkills);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          skills: _testSkills,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/check');
      await tester.pump();
      // First Enter fills the popup completion; the second submits the turn.
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      expect(provider.lastMessages!.first.role, ModelMessageRole.user);
      expect(
        textFromContent(provider.lastMessages!.first.content),
        contains('<skill>\n<name>check</name>'),
      );
    });
  });

  test('arrow keys move the popup selection and enter fills it in', () async {
    await testNocterm('slash popup selection', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      // Alphabetical order: compact, model, new, ... so two moves reach /new.
      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      // `/new ` replaces the token and the popup closes.
      expect(tester.terminalState, containsText('/new'));
      expect(tester.terminalState, isNot(containsText('/model')));
    });
  });

  test('escape dismisses the popup until the draft changes', () async {
    await testNocterm('slash popup dismiss', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/');
      await tester.pump();
      await tester.sendEscape();
      await tester.pump();
      expect(tester.terminalState, isNot(containsText('/new')));

      // The same draft stays dismissed; a change reopens the popup.
      await tester.sendEscape();
      await tester.pump();
      expect(tester.terminalState, isNot(containsText('/new')));
      await tester.enterText('n');
      await tester.pump();
      expect(tester.terminalState, containsText('/new'));
    });
  });

  test('/new clears the transcript and starts a fresh session', () async {
    await testNocterm('slash new', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('hello there');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      expect(tester.terminalState, containsText('hello there'));

      await tester.enterText('/new');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      expect(tester.terminalState, isNot(containsText('hello there')));
    });
  });

  test('/model opens the picker and enter switches the model', () async {
    await testNocterm('slash model', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/model');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();

      // The picker lists models by display name under a heading.
      expect(tester.terminalState, containsText('Select model'));
      expect(tester.terminalState, containsText('Alpha model'));
      expect(tester.terminalState, containsText('Beta model'));

      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      expect(tester.terminalState, containsText('Switched to Beta model'));
    });
  });

  test(
    '/model advances to the reasoning effort stage for multi-effort models',
    () async {
      await testNocterm('slash model effort', (tester) async {
        final provider = _ScriptedProvider();
        final runtime = _runtime(provider);
        await tester.pumpComponent(
          AtlasTuiApp(
            runtime: runtime,
            models: _reasoningModels,
            workingDirectory: '/tmp',
          ),
        );

        await tester.enterText('/model');
        await tester.sendEnter();
        await tester.pump();
        await tester.sendEnter();
        await tester.pump();

        // Select the first model (deep) and enter the effort stage.
        await tester.sendEnter();
        await tester.pump();
        expect(
          tester.terminalState,
          containsText('Select reasoning effort for Deep model'),
        );
        expect(tester.terminalState, containsText('Low effort'));
        expect(tester.terminalState, containsText('High effort'));

        await tester.sendArrowDown();
        await tester.pump();
        await tester.sendEnter();
        await tester.pump();

        expect(
          tester.terminalState,
          containsText('Switched to Deep model (fake), effort High effort'),
        );
      });
    },
  );

  test('/model applies a single-effort model directly', () async {
    await testNocterm('slash model single effort', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _reasoningModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/model');
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      // Move to the second model (single) and confirm: no effort stage.
      await tester.sendArrowDown();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();

      expect(
        tester.terminalState,
        containsText('Switched to Single model (fake), effort Medium'),
      );
      expect(
        tester.terminalState,
        isNot(containsText('Select reasoning effort')),
      );
    });
  });

  test('/model escape cancels without switching', () async {
    await testNocterm('slash model cancel', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/model');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEscape();
      await tester.pump();

      expect(tester.terminalState, isNot(containsText('Switched to')));
    });
  });

  test('/resume opens the picker and resumes the selected session', () async {
    await testNocterm('slash resume', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      // Create a session worth resuming.
      await tester.enterText('hello there');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      // /resume opens the picker with the session listed.
      await tester.enterText('/resume');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();

      expect(tester.terminalState, containsText('Resume session'));
      expect(tester.terminalState, containsText('hello there'));

      // The first row is the session; confirm directly.
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();

      expect(tester.terminalState, containsText('Resumed: hello there'));
    });
  });

  test('/resume escape cancels without resuming', () async {
    await testNocterm('slash resume cancel', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('hello there');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      await tester.enterText('/resume');
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      expect(tester.terminalState, containsText('Resume session'));

      await tester.sendEscape();
      await tester.pump();

      expect(tester.terminalState, isNot(containsText('Resume session')));
      expect(tester.terminalState, isNot(containsText('Resumed:')));
    });
  });

  test('/resume failure shows an error without a success notice', () async {
    await testNocterm('slash resume failure', (tester) async {
      final store = DriftSessionStore.inMemory();
      final provider = _ScriptedProvider();
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: LocalToolRegistry(const []),
        ids: SecureIdGenerator(),
        defaultModel: ModelRef(
          providerId: ProviderId('fake'),
          modelId: ModelId('model'),
        ),
      );
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      // Create a session worth resuming.
      await tester.enterText('hello there');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      // Open the picker, then delete the session before confirming.
      await tester.enterText('/resume');
      await tester.sendEnter();
      await tester.pump();
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      expect(tester.terminalState, containsText('Resume session'));

      final page = await store.listSessions(SessionQuery());
      await store.deleteSession(page.items.single.id);

      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();

      // The failure surfaces as an error message, not a success notice.
      expect(tester.terminalState, containsText('Session not found'));
      expect(tester.terminalState, isNot(containsText('Resumed:')));
    });
  });

  test('renders structured headings for known tools', () async {
    await testNocterm('tool headings', (tester) async {
      final provider = _ScriptedProvider()
        ..toolCalls = [
          ToolCall(
            id: ToolCallId('call-1'),
            name: 'shell',
            arguments: {'command': 'ls -la'},
          ),
          ToolCall(
            id: ToolCallId('call-2'),
            name: 'read',
            arguments: {'path': '/tmp/a.dart', 'offset': 10, 'limit': 50},
          ),
          ToolCall(
            id: ToolCallId('call-3'),
            name: 'edit',
            arguments: {
              'path': '/tmp/b.dart',
              'edits': [
                {'old_text': 'x', 'new_text': 'y'},
                {'old_text': 'p', 'new_text': 'q'},
              ],
            },
          ),
          ToolCall(
            id: ToolCallId('call-4'),
            name: 'write',
            arguments: {'path': '/tmp/c.dart', 'content': 'l1\nl2\nl3'},
          ),
        ];
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('go');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      await tester.pump();

      final text = tester.terminalState.getText();
      expect(text, contains('Shell(ls -la)'));
      expect(text, isNot(contains('\$ ls -la')));
      expect(text, contains('Read(/tmp/a.dart)'));
      expect(text, contains('offset 10, limit 50'));
      // Read results are file contents and stay out of the transcript.
      expect(text, isNot(contains('unknown tool: read')));
      expect(text, contains('Edit(/tmp/b.dart)'));
      expect(text, contains('2 text blocks'));
      expect(text, contains('Write(/tmp/c.dart)'));
      expect(text, contains('3 lines'));
    });
  });

  test('/quit invokes the quit callback', () async {
    await testNocterm('slash quit', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      var quitCount = 0;
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
          onQuit: () => quitCount++,
        ),
      );

      await tester.enterText('/quit');
      await tester.sendEnter();
      await tester.pump();
      // First Enter fills the popup completion; the second submits the command.
      await tester.sendEnter();
      await tester.pump();
      expect(quitCount, 1);
    });
  });

  test('unknown commands submit as normal messages', () async {
    await testNocterm('slash unknown', (tester) async {
      final provider = _ScriptedProvider();
      final runtime = _runtime(provider);
      await tester.pumpComponent(
        AtlasTuiApp(
          runtime: runtime,
          models: _testModels,
          workingDirectory: '/tmp',
        ),
      );

      await tester.enterText('/nope');
      await tester.sendEnter();
      await tester.pump();
      await Future<void>.delayed(const Duration(milliseconds: 30));
      await tester.pump();
      expect(provider.streamCalls, 1);
    });
  });
}

AgentRuntime _runtime(_ScriptedProvider provider, {SkillCatalog? skills}) =>
    AgentRuntime(
      store: DriftSessionStore.inMemory(),
      provider: provider,
      tools: LocalToolRegistry(const []),
      ids: SecureIdGenerator(),
      defaultModel: ModelRef(
        providerId: ProviderId('fake'),
        modelId: ModelId('model'),
      ),
      sessionContextBuilder: _contextBuilder(skills),
    );

/// A session context builder that injects [skills] for every directory.
SessionContext Function(String) _contextBuilder(SkillCatalog? skills) =>
    (cwd) => SessionContext(
      workingDirectory: cwd,
      instructions: const [],
      skills: skills ?? _TestSkillCatalog(const []),
    );

/// Shared test skills shown in the `/` completion popup.
final _testSkills = _TestSkillCatalog(const [
  Skill(
    name: 'check',
    description: 'Review code.',
    dir: '/skills/check',
    path: '/skills/check/SKILL.md',
    content: '# Check\n\nReview the diff.',
  ),
]);

/// Shared test models for `/model` switching.
final _testModels = [
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('alpha')),
    name: 'Alpha model',
  ),
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('beta')),
    name: 'Beta model',
  ),
];

/// Models exercising the reasoning-effort stages of `/model`.
final _reasoningModels = [
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('deep')),
    name: 'Deep model',
    reasoningEfforts: const [
      ReasoningEffortOption(value: 'low', name: 'Low effort'),
      ReasoningEffortOption(value: 'high', name: 'High effort'),
    ],
  ),
  ModelDescriptor(
    ref: ModelRef(providerId: ProviderId('fake'), modelId: ModelId('single')),
    name: 'Single model',
    reasoningEfforts: const [
      ReasoningEffortOption(value: 'medium', name: 'Medium'),
    ],
  ),
];

/// Answers every turn with a finished response after streaming one delta.
final class _ScriptedProvider implements ModelProvider {
  int streamCalls = 0;
  Completer<void>? gate;
  List<ModelMessage>? lastMessages;

  /// Tool calls yielded by the first request, if any.
  List<ToolCall> toolCalls = const [];

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    streamCalls++;
    lastMessages = request.messages;
    if (gate != null) {
      final cancellation = request.cancellation;
      await Future.any<void>([
        gate!.future,
        cancellation?.whenCancelled ?? Future<void>.value(),
      ]);
      if (cancellation?.isCancelled == true) {
        throw const TurnCancelledException();
      }
    }
    if (streamCalls == 1 && toolCalls.isNotEmpty) {
      yield ModelCompletedEvent(
        ModelResponse(toolCalls: toolCalls, stopReason: StopReason.toolUse),
      );
      return;
    }
    yield const TextDeltaEvent('hi');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('hi')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

/// In-memory skill catalog for the slash completion tests.
final class _TestSkillCatalog implements SkillCatalog {
  _TestSkillCatalog(this.skills);

  final List<Skill> skills;

  @override
  List<SkillSummary> get summaries => [
    for (final skill in skills)
      SkillSummary(
        name: skill.name,
        path: skill.path,
        description: skill.description,
      ),
  ];

  @override
  Skill? lookup(String name) {
    for (final skill in skills) {
      if (skill.name == name) {
        return skill;
      }
    }
    return null;
  }
}

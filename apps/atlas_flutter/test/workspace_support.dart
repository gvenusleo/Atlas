import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:atlas_flutter/features/workspace/application/workspace_controller.dart';

final class FixedWorkingDirectory extends WorkspaceWorkingDirectory {
  FixedWorkingDirectory(this.path);

  final String path;

  @override
  String build() => path;
}

final class GatedFakeProvider implements ModelProvider {
  GatedFakeProvider(this.model);

  final ModelRef model;
  final reasoningGate = Completer<void>();
  final textGate = Completer<void>();

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    expect(request.model, model);
    yield const ReasoningDeltaEvent('first thought');
    await reasoningGate.future;
    yield const TextDeltaEvent('Hello from Atlas.');
    await textGate.future;
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class FakeProvider implements ModelProvider {
  FakeProvider(this.model);

  final ModelRef model;

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async =>
      ModelDescriptor(ref: requested);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    expect(request.model, model);
    yield const ReasoningDeltaEvent('first thought');
    yield const TextDeltaEvent('Hello ');
    yield const TextDeltaEvent('from Atlas.');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
        reasoning: 'first thought',
      ),
    );
  }
}

final class EmptySkillCatalog implements SkillCatalog {
  @override
  Skill? lookup(String name) => null;

  @override
  List<SkillSummary> get summaries => const [];
}

/// A runtime that advertises agent modes and records mode switches.
final class FakeModeRuntime implements PresentationAgentSession {
  FakeModeRuntime(this._inner);

  final AgentRuntime _inner;
  final modeCalls = <(String, String)>[];
  final turnModes = <String?>[];

  static const modes = [
    ModeOption(id: 'build', name: 'build'),
    ModeOption(id: 'plan', name: 'plan'),
  ];

  @override
  ModelRef get defaultModel => _inner.defaultModel;

  @override
  Stream<AgentEvent> run(TurnRequest request) {
    turnModes.add(request.mode);
    return _inner.run(request);
  }

  @override
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    CancellationToken? cancellation,
  }) => _inner.compact(
    sessionId,
    instruction: instruction,
    cancellation: cancellation,
  );

  @override
  Future<SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  }) => _inner.listSessions(
    workingDirectory: workingDirectory,
    cursor: cursor,
    limit: limit,
  );

  @override
  Future<Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const <String>[],
  }) => _inner.createSession(
    workingDirectory: workingDirectory,
    additionalDirectories: additionalDirectories,
  );

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      _inner.loadSession(sessionId);

  @override
  Future<void> deleteSession(SessionId sessionId) =>
      _inner.deleteSession(sessionId);

  @override
  Future<void> renameSession(SessionId sessionId, String title) =>
      _inner.renameSession(sessionId, title);

  @override
  Future<int> contextWindowSize() => _inner.contextWindowSize();

  @override
  String? titleFor(SessionId sessionId) => _inner.titleFor(sessionId);

  @override
  List<AgentCommand> commandsFor(SessionId sessionId) =>
      _inner.commandsFor(sessionId);

  @override
  List<ModeOption> get modeOptions => modes;

  @override
  String? modeFor(SessionId sessionId) => null;

  @override
  Future<void> setMode(SessionId sessionId, String modeId) async {
    modeCalls.add((sessionId.value, modeId));
  }
}

final class RecordingProvider implements ModelProvider {
  final models = <ModelRef>[];
  final efforts = <String?>[];
  final contents = <List<ContentPart>>[];

  @override
  Future<ModelDescriptor> describe(ModelRef requested) async => ModelDescriptor(
    ref: requested,
    inputCapabilities: const {
      ModelInputCapability.text,
      ModelInputCapability.image,
    },
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    models.add(request.model);
    efforts.add(request.reasoningEffort);
    contents.add(request.messages.first.content);
    yield const TextDeltaEvent('ok');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('ok')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

final class BlockingProvider implements ModelProvider {
  BlockingProvider(this.model);

  final ModelRef model;
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
    yield const TextDeltaEvent('Hello from Atlas.');
    yield const ModelCompletedEvent(
      ModelResponse(
        content: [TextContent('Hello from Atlas.')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

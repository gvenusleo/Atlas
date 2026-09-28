import 'package:atlas_flutter/features/terminal/domain/terminal_port.dart';

import 'dart:async';
import 'dart:ui' show AppExitResponse;

import 'package:atlas_flutter/app/atlas_app.dart';
import 'package:atlas_flutter/features/connections/application/runtime_controller.dart';
import 'package:atlas_flutter/features/connections/domain/runtime_environment.dart';
import 'package:atlas_flutter/features/terminal/application/terminal_registry.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('an exit request kills the tracked shells and exits', (
    tester,
  ) async {
    await tester.pumpWidget(const ProviderScope(child: AtlasApp()));
    await tester.pump();
    final container = ProviderScope.containerOf(
      tester.element(find.byType(AtlasApp)),
    );
    final registry = container.read(terminalSessionRegistryProvider);
    final shell = _RecordingHandle();
    registry.track(shell);

    // The embedder asks before it terminates; a live PTY must be released here
    // or the isolate shuts down with a blocking `close` on its master.
    final response = await tester.binding.handleRequestAppExit();

    expect(response, AppExitResponse.exit);
    expect(shell.killCount, 1);
    expect(registry.isEmpty, isTrue);
  });

  testWidgets('an exit request releases the active runtime before exiting', (
    tester,
  ) async {
    var closed = 0;
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          runtimeEnvironmentProvider.overrideWith(
            () => RuntimeEnvironmentController(
              local: _exitEnvironment(() => closed++),
            ),
          ),
        ],
        child: const AtlasApp(),
      ),
    );
    await tester.pump();

    final response = await tester.binding.handleRequestAppExit();

    expect(response, AppExitResponse.exit);
    expect(closed, 1);
  });

  testWidgets('a runtime that never closes still lets the app exit', (
    tester,
  ) async {
    final never = Completer<void>();
    addTearDown(() {
      if (!never.isCompleted) never.complete();
    });
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          runtimeEnvironmentProvider.overrideWith(
            () => RuntimeEnvironmentController(
              local: _exitEnvironment(() {}, close: () => never.future),
            ),
          ),
        ],
        child: const AtlasApp(),
      ),
    );
    await tester.pump();

    final pending = tester.binding.handleRequestAppExit();
    await tester.pump(const Duration(seconds: 5));
    final response = await pending;

    expect(response, AppExitResponse.exit);
  });

  testWidgets('a shell that refuses to die still lets the app exit', (
    tester,
  ) async {
    await tester.pumpWidget(const ProviderScope(child: AtlasApp()));
    await tester.pump();
    final container = ProviderScope.containerOf(
      tester.element(find.byType(AtlasApp)),
    );
    final registry = container.read(terminalSessionRegistryProvider);
    registry.track(_ThrowingHandle());
    final healthy = _RecordingHandle();
    registry.track(healthy);

    final response = await tester.binding.handleRequestAppExit();

    expect(response, AppExitResponse.exit);
    expect(healthy.killCount, 1);
    expect(registry.isEmpty, isTrue);
  });
}

final class _RecordingHandle implements TerminalHandle {
  int killCount = 0;

  @override
  void kill() => killCount++;
}

/// A runtime environment whose close is observable, for the exit path.
RuntimeEnvironment _exitEnvironment(
  void Function() onClose, {
  Future<void> Function()? close,
}) => RuntimeEnvironment(
  runtime: _FakeSession(),
  models: const [],
  onClose: () async {
    onClose();
    await close?.call();
  },
);

final class _FakeSession implements PresentationAgentSession {
  @override
  ModelRef get defaultModel =>
      ModelRef(providerId: ProviderId('exit'), modelId: ModelId('none'));

  @override
  Stream<AgentEvent> run(TurnRequest request) => const Stream.empty();

  @override
  Stream<AgentEvent> compact(
    SessionId sessionId, {
    String? instruction,
    ModelRef? model,
    CancellationToken? cancellation,
  }) => const Stream.empty();

  @override
  Future<SessionPage> listSessions({
    String? workingDirectory,
    String? cursor,
    int limit = 20,
  }) async => const SessionPage(items: []);

  @override
  Future<Session> createSession({
    required String workingDirectory,
    List<String> additionalDirectories = const [],
  }) => throw UnimplementedError();

  @override
  Future<SessionSnapshot> loadSession(SessionId sessionId) =>
      throw UnimplementedError();

  @override
  Future<void> deleteSession(SessionId sessionId) async {}

  @override
  Future<void> renameSession(SessionId sessionId, String title) async {}

  @override
  Future<int> contextWindowSize({ModelRef? model}) async => 0;

  @override
  String? titleFor(SessionId sessionId) => null;

  @override
  List<AgentCommand> commandsFor(SessionId sessionId) => const [];

  @override
  List<ModeOption> get modeOptions => const [];

  @override
  String? modeFor(SessionId sessionId) => null;

  @override
  Future<void> setMode(SessionId sessionId, String modeId) async {}
}

final class _ThrowingHandle implements TerminalHandle {
  @override
  void kill() => throw StateError('kill failed');
}

import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'test_fakes.dart';

void main() {
  AgentRuntime runtime(MemorySessionStore store, ModelProvider provider) =>
      AgentRuntime(
        store: store,
        provider: provider,
        tools: ThrowingTools(),
        ids: TestIds(),
        defaultModel: testModel,
        keptRecentTurns: 1,
      );

  test(
    'shutdown cancels a turn without a caller token and drains persistence',
    () async {
      final store = MemorySessionStore();
      final provider = _ShutdownProvider();
      final engine = runtime(store, provider);
      final events = engine
          .run(
            const TurnRequest(
              content: [TextContent('hello')],
              workingDirectory: '/tmp',
            ),
          )
          .toList();
      await provider.started.future;

      var stopped = false;
      final stopping = engine.shutdown().then((_) => stopped = true);
      await provider.cancelled.future;
      expect(stopped, isFalse);
      provider.release.complete();
      await stopping;
      final result = await events;
      expect(
        result.whereType<TurnFinished>().single.outcome.status,
        TurnStatus.cancelled,
      );
      expect(store.turns.single.status, TurnStatus.cancelled);
      await engine.shutdown(); // Idempotent.
      await expectLater(
        engine.run(const TurnRequest(content: [])).toList(),
        throwsStateError,
      );
    },
  );

  test(
    'shutdown cancels queued turns without starting another model request',
    () async {
      final store = MemorySessionStore();
      final provider = _ShutdownProvider();
      final engine = runtime(store, provider);
      final session = await engine.createSession(workingDirectory: '/tmp');
      final first = engine
          .run(
            TurnRequest(
              sessionId: session.id,
              content: const [TextContent('first')],
            ),
          )
          .toList();
      await provider.started.future;
      final token = CancellationToken();
      final queued = expectLater(
        engine
            .run(
              TurnRequest(
                sessionId: session.id,
                content: const [TextContent('queued')],
                cancellation: token,
              ),
            )
            .toList(),
        throwsA(isA<TurnCancelledException>()),
      );
      // Let the async* subscription acquire its place behind the active turn.
      await Future<void>.delayed(Duration.zero);
      final stopping = engine.shutdown();
      await token.whenCancelled;
      provider.release.complete();
      await Future.wait([stopping, queued, first]);
      expect(provider.requests, 1);
      expect(store.turns, hasLength(1));
      expect(store.turns.single.status, TurnStatus.cancelled);
    },
  );

  test('shutdown cancels manual compaction before closing its store', () async {
    final store = MemorySessionStore();
    final seed = runtime(
      store,
      ScriptedProvider(
        List.filled(
          2,
          const ModelResponse(
            content: [TextContent('reply')],
            stopReason: StopReason.endTurn,
          ),
        ),
      ),
    );
    final first = await seed
        .run(
          const TurnRequest(
            content: [TextContent('first')],
            workingDirectory: '/tmp',
          ),
        )
        .toList();
    await seed
        .run(
          TurnRequest(
            sessionId: first.first.sessionId,
            content: const [TextContent('second')],
          ),
        )
        .drain<void>();
    await seed.shutdown();

    final provider = _ShutdownProvider();
    final engine = runtime(store, provider);
    final compacting = expectLater(
      engine.compact(first.first.sessionId).toList(),
      throwsA(isA<TurnCancelledException>()),
    );
    await provider.started.future;
    final stopping = engine.shutdown();
    await provider.cancelled.future;
    provider.release.complete();
    await Future.wait([stopping, compacting]);
    expect(store.compaction, isNull);
  });
}

class _ShutdownProvider implements ModelProvider {
  final started = Completer<void>();
  final cancelled = Completer<void>();
  final release = Completer<void>();
  int requests = 0;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async =>
      ModelDescriptor(ref: model, contextWindow: 10000);

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    requests++;
    started.complete();
    await request.cancellation!.whenCancelled;
    cancelled.complete();
    await release.future;
    request.cancellation!.throwIfCancelled();
  }
}

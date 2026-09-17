import 'dart:async';

import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'test_fakes.dart';

void main() {
  test(
    'emits live output in order and persists only the paired final result',
    () async {
      final tool = _StreamingTools();
      final store = MemorySessionStore();
      final provider = _provider();
      final runtime = _runtime(tool, store, provider);
      final iterator = StreamIterator(runtime.run(_request));
      final events = <AgentEvent>[];
      while (await iterator.moveNext()) {
        final event = iterator.current;
        events.add(event);
        if (event is ToolOutputUpdated) {
          expect(tool.finished, isFalse);
          expect(store.timeline.whereType<ToolResultItem>(), isEmpty);
          tool.release.complete();
        }
      }
      final start = events.indexWhere((e) => e is ToolStarted);
      final progress = events.indexWhere((e) => e is ToolOutputUpdated);
      final finish = events.indexWhere((e) => e is ToolFinished);
      expect(start, lessThan(progress));
      expect(progress, lessThan(finish));
      expect(
        events.map((e) => e.sequence),
        orderedEquals(List.generate(events.length, (i) => i)),
      );
      expect(
        store.timeline.whereType<ToolResultItem>().single.content,
        'final',
      );
      expect(provider.requests.last.messages.last.toolOutput, 'final');
      expect(provider.requests, hasLength(2));
    },
  );

  test('coalesces a paused consumer to one latest output snapshot', () async {
    final tool = _StreamingTools();
    final runtime = _runtime(tool, MemorySessionStore(), _provider());
    final iterator = StreamIterator(runtime.run(_request));
    while (await iterator.moveNext()) {
      if (iterator.current is ToolOutputUpdated) break;
    }
    for (var i = 0; i < 10000; i++) {
      tool.emit('output $i');
    }
    tool.release.complete();
    final remaining = <AgentEvent>[];
    while (await iterator.moveNext()) {
      remaining.add(iterator.current);
    }
    final outputs = remaining.whereType<ToolOutputUpdated>().toList();
    expect(outputs, hasLength(1));
    expect(outputs.single.output.content, 'output 9999');
    expect(remaining.whereType<ToolFinished>(), hasLength(1));
    // A tool retaining its callback cannot emit after the final result.
    tool.emit('late output');
  });

  test('cancels an active tool when its event consumer disconnects', () async {
    final tool = _StreamingTools();
    final store = MemorySessionStore();
    final provider = _provider();
    final runtime = _runtime(tool, store, provider);
    final iterator = StreamIterator(runtime.run(_request));
    while (await iterator.moveNext()) {
      if (iterator.current is ToolOutputUpdated) break;
    }
    await iterator.cancel().timeout(const Duration(seconds: 3));
    expect(tool.finished, isTrue);
    final result = store.timeline.whereType<ToolResultItem>().single;
    expect(result.content, 'final');
    expect(result.isError, isTrue);
    expect(store.turns.single.status, TurnStatus.cancelled);
    expect(provider.requests, hasLength(1));
  });

  for (final eventType in [
    TurnStarted,
    ModelResponseReceived,
    ToolStarted,
    ToolFinished,
  ]) {
    test(
      'disconnect at $eventType leaves a terminal turn and paired calls',
      () async {
        final store = MemorySessionStore();
        final tool = _StreamingTools()..release.complete();
        final provider = ScriptedProvider([
          ModelResponse(
            toolCalls: [
              for (final id in ['one', 'two'])
                ToolCall(
                  id: ToolCallId(id),
                  name: 'shell',
                  arguments: const {},
                ),
            ],
            stopReason: StopReason.toolUse,
          ),
        ]);
        final runtime = _runtime(tool, store, provider);
        final iterator = StreamIterator(runtime.run(_request));
        while (await iterator.moveNext()) {
          if (iterator.current.runtimeType == eventType) break;
        }
        await iterator.cancel().timeout(const Duration(seconds: 3));
        expect(store.turns.single.status, TurnStatus.cancelled);
        final calls = store.timeline.whereType<ToolCallItem>();
        final results = store.timeline.whereType<ToolResultItem>();
        expect(
          results.map((item) => item.callId),
          orderedEquals(calls.map((item) => item.call.id)),
        );
      },
    );
  }

  test('cancellation retains output and persists one error result', () async {
    final cancellation = CancellationToken();
    final tool = _StreamingTools();
    final store = MemorySessionStore();
    final runtime = _runtime(tool, store, _provider());
    final events = <AgentEvent>[];
    await for (final event in runtime.run(
      TurnRequest(
        workingDirectory: '/tmp',
        content: const [TextContent('run')],
        cancellation: cancellation,
      ),
    )) {
      events.add(event);
      if (event is ToolOutputUpdated) cancellation.cancel();
    }
    expect(events.whereType<ToolFinished>().single.result.isError, isTrue);
    expect(
      events.whereType<TurnFinished>().single.outcome.status,
      TurnStatus.cancelled,
    );
    expect(store.timeline.whereType<ToolResultItem>(), hasLength(1));
  });
}

const _request = TurnRequest(
  workingDirectory: '/tmp',
  content: [TextContent('run')],
);

ScriptedProvider _provider() => ScriptedProvider([
  ModelResponse(
    toolCalls: [
      ToolCall(id: ToolCallId('call'), name: 'shell', arguments: const {}),
    ],
    stopReason: StopReason.toolUse,
  ),
  const ModelResponse(
    content: [TextContent('done')],
    stopReason: StopReason.endTurn,
  ),
]);

AgentRuntime _runtime(
  ToolRegistry tools,
  MemorySessionStore store,
  ScriptedProvider provider,
) => AgentRuntime(
  tools: tools,
  store: store,
  provider: provider,
  ids: TestIds(),
  defaultModel: testModel,
);

final class _StreamingTools implements ToolRegistry {
  final release = Completer<void>();
  ToolContext? _context;
  bool finished = false;
  @override
  List<ToolDescriptor> get descriptors => const [];

  void emit(String content) => _context!.onOutput!(
    ToolOutputSnapshot(
      content: content,
      totalBytes: content.length,
      truncated: false,
    ),
  );

  @override
  Future<ToolResult> execute(ToolContext context, ToolCall call) async {
    _context = context;
    emit('live');
    await Future.any([release.future, context.cancellation!.whenCancelled]);
    finished = true;
    return ToolResult(
      content: 'final',
      isError: context.cancellation!.isCancelled,
    );
  }
}

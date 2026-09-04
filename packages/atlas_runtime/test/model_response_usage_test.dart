import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'test_fakes.dart';

void main() {
  test(
    'ModelResponseReceived carries the step usage for mid-turn updates',
    () async {
      final store = MemorySessionStore();
      final provider = ScriptedProvider([
        ModelResponse(
          content: const [TextContent('I will inspect the files.')],
          toolCalls: [
            ToolCall(
              id: ToolCallId('call-1'),
              name: 'inspect',
              arguments: <String, Object?>{'path': '.'},
            ),
          ],
          stopReason: StopReason.toolUse,
          usage: const TokenUsage(
            inputTokens: 700,
            outputTokens: 20,
            totalTokens: 720,
          ),
        ),
        const ModelResponse(
          content: [TextContent('Done.')],
          stopReason: StopReason.endTurn,
          usage: TokenUsage(
            inputTokens: 1500,
            outputTokens: 5,
            totalTokens: 1505,
          ),
        ),
      ]);
      final runtime = AgentRuntime(
        store: store,
        provider: provider,
        tools: MemoryTools(result: const ToolResult(content: 'file list')),
        ids: TestIds(),
        defaultModel: testModel,
      );

      final responses = await runtime
          .run(
            const TurnRequest(
              content: [TextContent('Inspect the files')],
              workingDirectory: '/tmp',
            ),
          )
          .where((event) => event is ModelResponseReceived)
          .cast<ModelResponseReceived>()
          .toList();

      expect(responses, hasLength(2));
      expect(responses[0].usage.inputTokens, 700);
      expect(responses[1].usage.inputTokens, 1500);
    },
  );
}

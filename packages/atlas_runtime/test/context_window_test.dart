import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

import 'test_fakes.dart';

/// A provider whose describe returns a distinct window per model ref.
final class _WindowProvider implements ModelProvider {
  @override
  Future<ModelDescriptor> describe(ModelRef model) async => ModelDescriptor(
    ref: model,
    name: 'test',
    contextWindow: model == testModel ? 1000 : 2000,
  );

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {}
}

void main() {
  test('contextWindowSize describes the requested model', () async {
    final runtime = AgentRuntime(
      store: MemorySessionStore(),
      provider: _WindowProvider(),
      tools: MemoryTools(result: const ToolResult(content: 'unused')),
      ids: TestIds(),
      defaultModel: testModel,
    );
    final otherModel = ModelRef(
      providerId: ProviderId('test'),
      modelId: ModelId('other'),
    );

    expect(await runtime.contextWindowSize(), 1000);
    expect(await runtime.contextWindowSize(model: otherModel), 2000);
  });
}

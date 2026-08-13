import 'package:atlas_provider/atlas_provider.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  test('routes requests to the provider that owns the model', () async {
    final openaiCalls = <String>[];
    final anthropicCalls = <String>[];
    final composite = CompositeModelProvider({
      ProviderId('openai'): _RecordingProvider(openaiCalls),
      ProviderId('anthropic'): _RecordingProvider(anthropicCalls),
    });

    await composite.describe(_model(ProviderId('openai'), 'gpt'));
    await composite
        .stream(_request(ProviderId('anthropic'), 'claude'))
        .toList();
    await composite.describe(_model(ProviderId('anthropic'), 'claude'));

    expect(openaiCalls, ['describe']);
    expect(anthropicCalls, ['stream', 'describe']);
  });

  test('reports an unconfigured provider as a failed event', () async {
    final composite = CompositeModelProvider({
      ProviderId('openai'): _RecordingProvider([]),
    });

    final events = await composite
        .stream(_request(ProviderId('missing'), 'model'))
        .toList();
    expect(events.single, isA<ModelFailedEvent>());
  });
}

ModelRef _model(ProviderId providerId, String modelId) =>
    ModelRef(providerId: providerId, modelId: ModelId(modelId));

ModelRequest _request(ProviderId providerId, String modelId) => ModelRequest(
  sessionId: SessionId('session-1'),
  turnId: TurnId('turn-1'),
  model: _model(providerId, modelId),
  messages: const [
    ModelMessage(role: ModelMessageRole.user, content: [TextContent('Hi')]),
  ],
);

final class _RecordingProvider implements ModelProvider {
  _RecordingProvider(this.calls);

  final List<String> calls;

  @override
  Future<ModelDescriptor> describe(ModelRef model) async {
    calls.add('describe');
    return ModelDescriptor(ref: model);
  }

  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) async* {
    calls.add('stream');
    yield ModelCompletedEvent(
      ModelResponse(
        content: const [TextContent('ok')],
        stopReason: StopReason.endTurn,
      ),
    );
  }
}

import '../domain/ids.dart';
import '../domain/model.dart';
import 'cancellation.dart';

/// A model request assembled by the runtime.
final class ModelRequest {
  /// Creates a model request.
  const ModelRequest({
    required this.sessionId,
    required this.turnId,
    required this.model,
    required this.messages,
    this.systemPrompt = '',
    this.tools = const <ToolDescriptor>[],
    this.reasoningEffort,
    this.maxOutputTokens = 0,
    this.temperature,
    this.responseFormat,
    this.cancellation,
  });

  /// The session correlation identifier.
  final SessionId sessionId;

  /// The turn correlation identifier.
  final TurnId turnId;

  /// The selected model.
  final ModelRef model;

  /// The provider-neutral model context.
  final List<ModelMessage> messages;

  /// The generated system prompt.
  final String systemPrompt;

  /// Tools available to the model.
  final List<ToolDescriptor> tools;

  /// The selected provider-local reasoning effort.
  final String? reasoningEffort;

  /// The output token limit.
  final int maxOutputTokens;

  /// The sampling temperature.
  final double? temperature;

  /// The requested structured response format.
  final String? responseFormat;

  /// Cooperative cancellation for the provider stream.
  final CancellationToken? cancellation;
}

/// An incremental event from a model provider.
sealed class ModelStreamEvent {
  /// Creates a provider stream event.
  const ModelStreamEvent();
}

/// An assistant text fragment.
final class TextDeltaEvent extends ModelStreamEvent {
  /// Creates a text delta event.
  const TextDeltaEvent(this.delta);

  /// The new text fragment.
  final String delta;
}

/// A reasoning summary fragment.
final class ReasoningDeltaEvent extends ModelStreamEvent {
  /// Creates a reasoning delta event.
  const ReasoningDeltaEvent(this.delta);

  /// The new reasoning fragment.
  final String delta;
}

/// A completed model response.
final class ModelCompletedEvent extends ModelStreamEvent {
  /// Creates a completed model event.
  const ModelCompletedEvent(this.response);

  /// The accumulated response.
  final ModelResponse response;
}

/// A provider-reported failure.
final class ModelFailedEvent extends ModelStreamEvent {
  /// Creates a failed model event.
  const ModelFailedEvent(this.error, this.stackTrace);

  /// The provider error.
  final Object error;

  /// The error stack trace.
  final StackTrace stackTrace;
}

/// Provides model catalog information and streaming model responses.
abstract interface class ModelProvider {
  /// Returns the descriptor for [model].
  Future<ModelDescriptor> describe(ModelRef model);

  /// Streams one model step.
  Stream<ModelStreamEvent> stream(ModelRequest request);
}

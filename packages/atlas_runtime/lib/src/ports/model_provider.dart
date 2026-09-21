import '../domain/ids.dart';
import '../domain/model.dart';
import 'cancellation.dart';

/// A model request assembled by the runtime.
final class const ModelRequest({
  /// The session correlation identifier.
  required final SessionId sessionId,

  /// The turn correlation identifier.
  required final TurnId turnId,

  /// The selected model.
  required final ModelRef model,

  /// The provider-neutral model context.
  required final List<ModelMessage> messages,

  /// The generated system prompt.
  final String systemPrompt = '',

  /// Tools available to the model.
  final List<ToolDescriptor> tools = const <ToolDescriptor>[],

  /// The selected provider-local reasoning effort.
  final String? reasoningEffort,

  /// The output token limit.
  final int maxOutputTokens = 0,

  /// The sampling temperature.
  final double? temperature,

  /// Provider-specific request fields passed through without interpretation.
  final Map<String, Object?> providerOptions = const <String, Object?>{},

  /// Cooperative cancellation for the provider stream.
  final CancellationToken? cancellation,
}) {
  /// Creates a model request.
  this;
}

/// An incremental event from a model provider.
sealed class const ModelStreamEvent() {
  /// Creates a provider stream event.
  this;
}

/// An assistant text fragment.
final class const TextDeltaEvent(
  /// The new text fragment.
  final String delta,
) extends ModelStreamEvent {
  /// Creates a text delta event.
  this;
}

/// A reasoning summary fragment.
final class const ReasoningDeltaEvent(
  /// The new reasoning fragment.
  final String delta,
) extends ModelStreamEvent {
  /// Creates a reasoning delta event.
  this;
}

/// A completed model response.
final class const ModelCompletedEvent(
  /// The accumulated response.
  final ModelResponse response,
) extends ModelStreamEvent {
  /// Creates a completed model event.
  this;
}

/// A provider-reported failure.
final class const ModelFailedEvent(
  /// The provider error.
  final Object error,

  /// The error stack trace.
  final StackTrace stackTrace,
) extends ModelStreamEvent {
  /// Creates a failed model event.
  this;
}

/// Provides model catalog information and streaming model responses.
abstract interface class ModelProvider {
  /// Returns the descriptor for [model].
  Future<ModelDescriptor> describe(ModelRef model);

  /// Streams one model step.
  Stream<ModelStreamEvent> stream(ModelRequest request);
}

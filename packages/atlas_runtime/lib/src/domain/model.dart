import 'content.dart';
import 'ids.dart';
import 'usage.dart';

/// An agent operating mode offered by a server.
final class const ModeOption({
  /// The mode identifier sent with a request.
  required final String id,

  /// Display name.
  required final String name,

  /// Display description.
  final String description = '',
}) {
  /// Creates a mode option.
  this;
}

/// An authentication method advertised by an agent.
final class const AuthMethod({
  /// The method identifier sent with an authenticate request.
  required final String id,

  /// Display name.
  required final String name,

  /// Display description.
  final String description = '',
}) {
  /// Creates an auth method.
  this;
}

/// A reasoning effort supported by a model.
final class const ReasoningEffortOption({
  /// Provider-local value sent with a request.
  required final String value,

  /// Display name.
  final String name = '',

  /// Display description.
  final String description = '',
}) {
  /// Creates a reasoning effort option.
  this;
}

/// Input capabilities exposed by a model.
enum ModelInputCapability {
  /// Plain text input.
  text,

  /// Image input.
  image,
}

/// A configured model and its capabilities.
final class const ModelDescriptor({
  /// The provider/model reference.
  required final ModelRef ref,

  /// Display name.
  final String name = '',

  /// Display description.
  final String description = '',

  /// Maximum context window in tokens.
  final int contextWindow = 0,

  /// Maximum output tokens.
  final int maxOutputTokens = 0,

  /// Modalities accepted by the model.
  final Set<ModelInputCapability> inputCapabilities =
      const <ModelInputCapability>{ModelInputCapability.text},

  /// Reasoning effort values accepted by the model.
  final List<ReasoningEffortOption> reasoningEfforts =
      const <ReasoningEffortOption>[],
}) {
  /// Creates a model descriptor.
  this;
}

/// The reason a model step stopped.
enum StopReason {
  /// The model produced a terminal response.
  endTurn,

  /// The model requested one or more tools.
  toolUse,

  /// The output token limit stopped generation.
  maxTokens,

  /// The stream was interrupted before a terminal response; the persisted
  /// content is the partial answer received so far.
  aborted,

  /// The provider did not expose a recognized reason.
  unknown,
}

/// A model-requested tool invocation.
final class const ToolCall({
  /// The provider-generated call identifier.
  required final ToolCallId id,

  /// The registered tool name.
  required final String name,

  /// The parsed JSON arguments.
  required final JsonObject arguments,
}) {
  /// Creates a tool call.
  this;
}

/// A model-visible role after timeline projection.
enum ModelMessageRole {
  /// User input.
  user,

  /// Assistant output or tool requests.
  assistant,

  /// A tool result.
  tool,
}

/// A provider-neutral message projected from the durable timeline.
final class const ModelMessage({
  /// The model role.
  required final ModelMessageRole role,

  /// Structured text and image content.
  final List<ContentPart> content = const <ContentPart>[],

  /// Tool calls emitted by an assistant message.
  final List<ToolCall> toolCalls = const <ToolCall>[],

  /// The matching call ID for a tool result.
  final ToolCallId? toolCallId,

  /// The tool output for a tool result.
  final String? toolOutput,

  /// Provider-owned continuation attached to this assistant message.
  final ModelContinuation? continuation,
}) {
  /// Creates a model message.
  this;
}

/// A tool schema advertised to the model.
final class const ToolDescriptor({
  /// The unique model-facing tool name.
  required final String name,

  /// The tool's model-facing description.
  required final String description,

  /// A JSON Schema object for tool arguments.
  required final JsonObject inputSchema,
}) {
  /// Creates a tool descriptor.
  this;
}

/// A provider's opaque continuation and optional reasoning summary.
final class const ModelContinuation({
  /// The provider that owns [opaquePayload].
  required final ProviderId providerId,

  /// Provider-produced reasoning summary, if available.
  final String reasoningSummary = '',

  /// Provider-owned continuation payload.
  final JsonObject opaquePayload = const <String, Object?>{},
}) {
  /// Creates a model continuation.
  this;
}

/// A persisted provider continuation linked to an assistant item.
final class const ModelCheckpoint({
  /// The assistant item that produced this checkpoint.
  required final TimelineItemId timelineItemId,

  /// The provider-owned continuation value.
  required final ModelContinuation continuation,

  /// The UTC time at which the checkpoint was persisted.
  required final DateTime createdAt,
}) {
  /// Creates a model checkpoint.
  this;
}

/// A result returned by a completed model step.
final class const ModelResponse({
  /// The assistant content.
  final List<ContentPart> content = const [],

  /// Tool calls requested by the assistant.
  final List<ToolCall> toolCalls = const [],

  /// Why the model step stopped.
  final StopReason stopReason = StopReason.unknown,

  /// Provider-neutral reasoning text produced before the response.
  final String reasoning = '',

  /// Token usage for this step.
  final TokenUsage usage = const TokenUsage(),

  /// Continuation state for this response.
  final ModelContinuation? continuation,
}) {
  /// Creates a model response.
  this;
}

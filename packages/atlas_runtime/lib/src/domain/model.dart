import 'content.dart';
import 'ids.dart';
import 'usage.dart';

/// A reasoning effort supported by a model.
final class ReasoningEffortOption {
  /// Creates a reasoning effort option.
  const ReasoningEffortOption({
    required this.value,
    this.name = '',
    this.description = '',
  });

  /// Provider-local value sent with a request.
  final String value;

  /// Display name.
  final String name;

  /// Display description.
  final String description;
}

/// Input capabilities exposed by a model.
enum ModelInputCapability {
  /// Plain text input.
  text,

  /// Image input.
  image,
}

/// A configured model and its capabilities.
final class ModelDescriptor {
  /// Creates a model descriptor.
  const ModelDescriptor({
    required this.ref,
    this.name = '',
    this.description = '',
    this.contextWindow = 0,
    this.maxOutputTokens = 0,
    this.inputCapabilities = const <ModelInputCapability>{
      ModelInputCapability.text,
    },
    this.reasoningEfforts = const <ReasoningEffortOption>[],
  });

  /// The provider/model reference.
  final ModelRef ref;

  /// Display name.
  final String name;

  /// Display description.
  final String description;

  /// Maximum context window in tokens.
  final int contextWindow;

  /// Maximum output tokens.
  final int maxOutputTokens;

  /// Modalities accepted by the model.
  final Set<ModelInputCapability> inputCapabilities;

  /// Reasoning effort values accepted by the model.
  final List<ReasoningEffortOption> reasoningEfforts;
}

/// The reason a model step stopped.
enum StopReason {
  /// The model produced a terminal response.
  endTurn,

  /// The model requested one or more tools.
  toolUse,

  /// The output token limit stopped generation.
  maxTokens,

  /// The provider did not expose a recognized reason.
  unknown,
}

/// A model-requested tool invocation.
final class ToolCall {
  /// Creates a tool call.
  const ToolCall({
    required this.id,
    required this.name,
    required this.arguments,
  });

  /// The provider-generated call identifier.
  final ToolCallId id;

  /// The registered tool name.
  final String name;

  /// The parsed JSON arguments.
  final JsonObject arguments;
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
final class ModelMessage {
  /// Creates a model message.
  const ModelMessage({
    required this.role,
    this.content = const <ContentPart>[],
    this.toolCalls = const <ToolCall>[],
    this.toolCallId,
    this.toolOutput,
    this.continuation,
  });

  /// The model role.
  final ModelMessageRole role;

  /// Structured text and image content.
  final List<ContentPart> content;

  /// Tool calls emitted by an assistant message.
  final List<ToolCall> toolCalls;

  /// The matching call ID for a tool result.
  final ToolCallId? toolCallId;

  /// The tool output for a tool result.
  final String? toolOutput;

  /// Provider-owned continuation attached to this assistant message.
  final ModelContinuation? continuation;
}

/// A tool schema advertised to the model.
final class ToolDescriptor {
  /// Creates a tool descriptor.
  const ToolDescriptor({
    required this.name,
    required this.description,
    required this.inputSchema,
  });

  /// The unique model-facing tool name.
  final String name;

  /// The tool's model-facing description.
  final String description;

  /// A JSON Schema object for tool arguments.
  final JsonObject inputSchema;
}

/// A provider's opaque continuation and optional reasoning summary.
final class ModelContinuation {
  /// Creates a model continuation.
  const ModelContinuation({
    required this.providerId,
    this.reasoningSummary = '',
    this.opaquePayload = const <String, Object?>{},
  });

  /// The provider that owns [opaquePayload].
  final ProviderId providerId;

  /// Provider-produced reasoning summary, if available.
  final String reasoningSummary;

  /// Provider-owned continuation payload.
  final JsonObject opaquePayload;
}

/// A persisted provider continuation linked to an assistant item.
final class ModelCheckpoint {
  /// Creates a model checkpoint.
  const ModelCheckpoint({
    required this.timelineItemId,
    required this.continuation,
    required this.createdAt,
  });

  /// The assistant item that produced this checkpoint.
  final TimelineItemId timelineItemId;

  /// The provider-owned continuation value.
  final ModelContinuation continuation;

  /// The UTC time at which the checkpoint was persisted.
  final DateTime createdAt;
}

/// A result returned by a completed model step.
final class ModelResponse {
  /// Creates a model response.
  const ModelResponse({
    this.content = const [],
    this.toolCalls = const [],
    this.stopReason = StopReason.unknown,
    this.usage = const TokenUsage(),
    this.continuation,
  });

  /// The assistant content.
  final List<ContentPart> content;

  /// Tool calls requested by the assistant.
  final List<ToolCall> toolCalls;

  /// Why the model step stopped.
  final StopReason stopReason;

  /// Token usage for this step.
  final TokenUsage usage;

  /// Continuation state for this response.
  final ModelContinuation? continuation;
}

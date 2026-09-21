import 'package:atlas_runtime/atlas_runtime.dart';

/// Configuration for the Anthropic Messages endpoint.
final class AnthropicProviderConfiguration({
  /// The provider identifier used by configured model references.
  required final ProviderId id,

  /// The API root, without the `/v1/messages` path.
  required final Uri baseUrl,

  /// The API key sent as `x-api-key`.
  required final String apiKey,
  required List<AnthropicModelConfiguration> models,

  /// The `anthropic-version` header value.
  final String apiVersion = '2023-06-01',

  /// An optional user-agent override.
  final String? userAgent,
}) {
  /// Creates a provider configuration.
  this : models = List<AnthropicModelConfiguration>.unmodifiable(models);

  /// Models served by this endpoint.
  final List<AnthropicModelConfiguration> models;
}

/// Configuration for one model exposed by an Anthropic provider.
final class const AnthropicModelConfiguration({
  /// The runtime model descriptor.
  required final ModelDescriptor descriptor,

  /// Extended thinking budget in tokens; zero disables thinking.
  final int thinkingBudgetTokens = 0,
}) {
  /// Creates a model configuration.
  this;
}

/// A safe provider failure with the endpoint identity and optional HTTP status.
final class const AnthropicProviderException({
  /// The provider that reported the failure.
  required final ProviderId providerId,

  /// A redacted, provider-safe error message.
  required final String message,

  /// The HTTP status when the server returned one.
  final int? statusCode,

  /// Bounded provider response detail for diagnostics.
  final String? detail,
}) implements SafeMessageException {
  /// Creates a provider failure.
  this;

  @override
  String get safeMessage => detail == null ? message : '$message: $detail';

  @override
  String? get diagnosticDetail => detail;

  @override
  String toString() {
    final status = statusCode == null ? '' : ' (status $statusCode)';
    return 'AnthropicProviderException[$providerId]$status: $message';
  }
}

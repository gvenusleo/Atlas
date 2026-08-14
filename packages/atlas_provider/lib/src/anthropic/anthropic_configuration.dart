import 'package:atlas_runtime/atlas_runtime.dart';

/// Configuration for the Anthropic Messages endpoint.
final class AnthropicProviderConfiguration {
  /// Creates a provider configuration.
  AnthropicProviderConfiguration({
    required this.id,
    required this.baseUrl,
    required this.apiKey,
    required List<AnthropicModelConfiguration> models,
    this.apiVersion = '2023-06-01',
    this.userAgent,
  }) : models = List<AnthropicModelConfiguration>.unmodifiable(models);

  /// The provider identifier used by configured model references.
  final ProviderId id;

  /// The API root, without the `/v1/messages` path.
  final Uri baseUrl;

  /// The API key sent as `x-api-key`.
  final String apiKey;

  /// The `anthropic-version` header value.
  final String apiVersion;

  /// An optional user-agent override.
  final String? userAgent;

  /// Models served by this endpoint.
  final List<AnthropicModelConfiguration> models;
}

/// Configuration for one model exposed by an Anthropic provider.
final class AnthropicModelConfiguration {
  /// Creates a model configuration.
  const AnthropicModelConfiguration({
    required this.descriptor,
    this.thinkingBudgetTokens = 0,
  });

  /// The runtime model descriptor.
  final ModelDescriptor descriptor;

  /// Extended thinking budget in tokens; zero disables thinking.
  final int thinkingBudgetTokens;
}

/// A safe provider failure with the endpoint identity and optional HTTP status.
final class AnthropicProviderException implements SafeMessageException {
  /// Creates a provider failure.
  const AnthropicProviderException({
    required this.providerId,
    required this.message,
    this.statusCode,
  });

  /// The provider that reported the failure.
  final ProviderId providerId;

  /// The HTTP status when the server returned one.
  final int? statusCode;

  /// A redacted, provider-safe error message.
  final String message;

  @override
  String get safeMessage => message;

  @override
  String toString() {
    final status = statusCode == null ? '' : ' (status $statusCode)';
    return 'AnthropicProviderException[$providerId]$status: $message';
  }
}

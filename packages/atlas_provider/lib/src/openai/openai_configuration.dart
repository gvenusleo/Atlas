import 'package:atlas_runtime/atlas_runtime.dart';

/// The streaming API variant exposed by an OpenAI-compatible endpoint.
enum OpenAIProtocol {
  /// The `/chat/completions` API.
  chatCompletions,

  /// The `/responses` API.
  responses,
}

/// Configuration for one OpenAI-compatible provider endpoint.
final class OpenAIProviderConfiguration {
  /// Creates a provider configuration.
  OpenAIProviderConfiguration({
    required this.id,
    required this.protocol,
    required this.baseUrl,
    required List<OpenAIModelConfiguration> models,
    this.apiKey = '',
    this.userAgent,
  }) : models = List<OpenAIModelConfiguration>.unmodifiable(models);

  /// The provider identifier used by configured model references.
  final ProviderId id;

  /// The API protocol used by this endpoint.
  final OpenAIProtocol protocol;

  /// The endpoint root, without the protocol-specific path.
  final Uri baseUrl;

  /// The bearer token. Empty values omit the authorization header.
  final String apiKey;

  /// An optional user-agent override.
  final String? userAgent;

  /// Models served by this endpoint.
  final List<OpenAIModelConfiguration> models;
}

/// Configuration for one model exposed by an OpenAI-compatible provider.
final class OpenAIModelConfiguration {
  /// Creates a model configuration.
  const OpenAIModelConfiguration({
    required this.descriptor,
    this.promptCacheEnabled = false,
  });

  /// The runtime model descriptor.
  final ModelDescriptor descriptor;

  /// Whether to send the session identifier as a prompt cache key.
  final bool promptCacheEnabled;
}

/// A safe provider failure with the endpoint identity and optional HTTP status.
final class OpenAIProviderException implements Exception {
  /// Creates a provider failure.
  const OpenAIProviderException({
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
  String toString() {
    final status = statusCode == null ? '' : ' (status $statusCode)';
    return 'OpenAIProviderException[$providerId]$status: $message';
  }
}

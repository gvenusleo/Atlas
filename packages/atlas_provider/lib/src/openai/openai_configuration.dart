import 'package:atlas_runtime/atlas_runtime.dart';

/// The streaming API variant exposed by an OpenAI-compatible endpoint.
enum OpenAIProtocol {
  /// The `/chat/completions` API.
  chatCompletions,

  /// The `/responses` API.
  responses,
}

/// Configuration for one OpenAI-compatible provider endpoint.
final class OpenAIProviderConfiguration({
  /// The provider identifier used by configured model references.
  required final ProviderId id,

  /// The API protocol used by this endpoint.
  required final OpenAIProtocol protocol,

  /// The endpoint root, without the protocol-specific path.
  required final Uri baseUrl,
  required List<OpenAIModelConfiguration> models,

  /// The bearer token. Empty values omit the authorization header.
  final String apiKey = '',

  /// An optional user-agent override.
  final String? userAgent,
}) {
  /// Creates a provider configuration.
  this : models = List<OpenAIModelConfiguration>.unmodifiable(models);

  /// Models served by this endpoint.
  final List<OpenAIModelConfiguration> models;
}

/// Configuration for one model exposed by an OpenAI-compatible provider.
final class const OpenAIModelConfiguration({
  /// The runtime model descriptor.
  required final ModelDescriptor descriptor,
}) {
  /// Creates a model configuration.
  this;
}

/// A safe provider failure with the endpoint identity and optional HTTP status.
final class const OpenAIProviderException({
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
    return 'OpenAIProviderException[$providerId]$status: $message';
  }
}

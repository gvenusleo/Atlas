import 'package:atlas_runtime/atlas_runtime.dart';

/// Routes model requests to the provider that owns each model reference.
final class CompositeModelProvider implements ModelProvider {
  /// Creates a composite from providers keyed by their provider identifier.
  CompositeModelProvider(Map<ProviderId, ModelProvider> providers)
    : _providers = Map<ProviderId, ModelProvider>.unmodifiable(providers);

  final Map<ProviderId, ModelProvider> _providers;

  /// Returns the descriptor from the provider that owns [model].
  @override
  Future<ModelDescriptor> describe(ModelRef model) {
    final provider = _providers[model.providerId];
    if (provider == null) {
      throw ArgumentError.value(
        model.providerId,
        'providerId',
        'is not configured',
      );
    }
    return provider.describe(model);
  }

  /// Streams the request through the provider that owns the requested model.
  @override
  Stream<ModelStreamEvent> stream(ModelRequest request) {
    final provider = _providers[request.model.providerId];
    if (provider == null) {
      return Stream<ModelStreamEvent>.value(
        ModelFailedEvent(
          ArgumentError.value(
            request.model.providerId,
            'providerId',
            'is not configured',
          ),
          StackTrace.current,
        ),
      );
    }
    return provider.stream(request);
  }
}

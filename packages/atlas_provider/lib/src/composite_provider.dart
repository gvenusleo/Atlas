import 'package:atlas_runtime/atlas_runtime.dart';

/// Routes each model to its API adapter, including mixed-protocol relays.
final class CompositeModelProvider(Map<ModelRef, ModelProvider> providers)
    implements ModelProvider {
  /// Creates a composite keyed by the complete provider/model identity.
  this : _providers = Map<ModelRef, ModelProvider>.unmodifiable(providers);

  final Map<ModelRef, ModelProvider> _providers;

  /// Returns the descriptor from the provider that owns [model].
  @override
  Future<ModelDescriptor> describe(ModelRef model) {
    final provider = _providers[model];
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
    final provider = _providers[request.model];
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

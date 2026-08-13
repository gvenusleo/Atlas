/// Opaque JSON-compatible values used by runtime boundaries.
typedef JsonValue = Object?;

/// A JSON object value.
typedef JsonObject = Map<String, Object?>;

/// Identifies a durable Atlas session.
final class SessionId {
  /// Creates a session identifier from a non-empty value.
  SessionId(String value) : value = _requireId(value);

  /// The serialized identifier.
  final String value;

  @override
  String toString() => value;

  @override
  bool operator ==(Object other) => other is SessionId && other.value == value;

  @override
  int get hashCode => value.hashCode;
}

/// Identifies one user turn within a session.
final class TurnId {
  /// Creates a turn identifier from a non-empty value.
  TurnId(String value) : value = _requireId(value);

  /// The serialized identifier.
  final String value;

  @override
  String toString() => value;

  @override
  bool operator ==(Object other) => other is TurnId && other.value == value;

  @override
  int get hashCode => value.hashCode;
}

/// Identifies one persisted timeline item.
final class TimelineItemId {
  /// Creates a timeline item identifier from a non-empty value.
  TimelineItemId(String value) : value = _requireId(value);

  /// The serialized identifier.
  final String value;

  @override
  String toString() => value;

  @override
  bool operator ==(Object other) =>
      other is TimelineItemId && other.value == value;

  @override
  int get hashCode => value.hashCode;
}

/// Identifies a model-requested tool call.
final class ToolCallId {
  /// Creates a tool call identifier from a non-empty value.
  ToolCallId(String value) : value = _requireId(value);

  /// The serialized identifier.
  final String value;

  @override
  String toString() => value;

  @override
  bool operator ==(Object other) => other is ToolCallId && other.value == value;

  @override
  int get hashCode => value.hashCode;
}

/// Identifies a configured model provider.
final class ProviderId {
  /// Creates a provider identifier from a non-empty value.
  ProviderId(String value) : value = _requireId(value);

  /// The serialized identifier.
  final String value;

  @override
  String toString() => value;

  @override
  bool operator ==(Object other) => other is ProviderId && other.value == value;

  @override
  int get hashCode => value.hashCode;
}

/// Identifies a model within a provider.
final class ModelId {
  /// Creates a model identifier from a non-empty value.
  ModelId(String value) : value = _requireId(value);

  /// The serialized identifier.
  final String value;

  @override
  String toString() => value;

  @override
  bool operator ==(Object other) => other is ModelId && other.value == value;

  @override
  int get hashCode => value.hashCode;
}

/// Identifies a model together with its owning provider.
final class ModelRef {
  /// Creates a model reference.
  const ModelRef({required this.providerId, required this.modelId});

  /// The provider that owns the model.
  final ProviderId providerId;

  /// The provider-local model identifier.
  final ModelId modelId;

  @override
  String toString() => '${providerId.value}/${modelId.value}';

  @override
  bool operator ==(Object other) =>
      other is ModelRef &&
      other.providerId == providerId &&
      other.modelId == modelId;

  @override
  int get hashCode => Object.hash(providerId, modelId);
}

/// Creates a deeply immutable JSON value for storage and transport boundaries.
JsonValue freezeJson(JsonValue value) {
  if (value is Map<String, Object?>) {
    return Map<String, Object?>.unmodifiable(
      value.map((key, nested) => MapEntry(key, freezeJson(nested))),
    );
  }
  if (value is List<Object?>) {
    return List<Object?>.unmodifiable(value.map(freezeJson));
  }
  return value;
}

/// Returns an immutable copy of a JSON object.
JsonObject immutableJsonObject(JsonObject value) =>
    freezeJson(value) as JsonObject;

String _requireId(String value) {
  if (value.isEmpty) {
    throw ArgumentError.value(value, 'value', 'must not be empty');
  }
  return value;
}

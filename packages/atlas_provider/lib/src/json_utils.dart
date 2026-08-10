/// Coerces a decoded JSON value into a string-keyed map, or an empty map.
Map<String, Object?> asJsonMap(Object? value) => value is Map
    ? value.map((key, value) => MapEntry(key.toString(), value))
    : <String, Object?>{};

/// Coerces a decoded JSON value into an integer, defaulting to zero.
int asInt(Object? value) => value is num ? value.toInt() : 0;

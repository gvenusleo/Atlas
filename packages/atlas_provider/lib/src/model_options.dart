/// Provider-owned Pi model options used when encoding a request.
final class const ModelOptions({
  /// Verified protocol compatibility flags for this model.
  final Map<String, Object?> compat = const {},

  /// Maps UI thinking levels onto protocol effort values.
  final Map<String, String?> thinkingLevelMap = const {},

  /// Model-specific sampling defaults.
  final Map<String, Object?> samplingParams = const {},

  /// Sampling overrides indexed by effective thinking level.
  final Map<String, Map<String, Object?>> samplingParamsByThinkingLevel =
      const {},

  /// Whether extended reasoning is supported.
  final bool reasoning = false,

  /// Startup preference, used when the request has no selection.
  final String? defaultThinkingLevel,

  /// Published USD prices per million tokens; omitted rates remain unknown.
  final Map<String, Object?> cost = const {},
}) {
  /// Creates immutable request defaults supplied by the configuration loader.
  this;

  /// Returns an explicit compatibility flag or its protocol default.
  bool flag(String name, bool fallback) => compat[name] as bool? ?? fallback;

  /// Resolves the request's thinking selection.
  String? level(String? requested) =>
      requested ?? (reasoning ? defaultThinkingLevel : null);

  /// Maps a resolved thinking level; null means omit the effort field.
  String? effort(String? level) => thinkingLevelMap.containsKey(level)
      ? thinkingLevelMap[level]
      : (level == 'off' ? null : level);

  /// Merges sampling defaults and the selected level's overrides.
  Map<String, Object?> sampling(String? level) => {
    ...samplingParams,
    ...?samplingParamsByThinkingLevel[level ?? 'off'],
  };
}

import 'package:atlas_runtime/atlas_runtime.dart';

/// One persisted turn's token usage, used for cache reporting.
final class const TurnUsageSample({
  /// Serialized identifier of the owning session.
  required final String sessionId,

  /// Serialized identifier of the turn.
  required final String turnId,

  /// Session title at read time.
  required final String title,

  /// UTC start time of the turn.
  required final DateTime startedAt,

  /// Provider that served the turn, when recorded.
  final String? providerId,

  /// Model that served the turn, when recorded.
  final String? modelId,

  /// Input tokens as reported by the provider.
  ///
  /// Anthropic reports this figure without the cached tokens, while
  /// OpenAI-compatible endpoints already include them.
  final int inputTokens = 0,

  /// Output tokens as reported by the provider.
  final int outputTokens = 0,

  /// Cached input tokens read from the provider cache.
  final int cacheReadTokens = 0,

  /// Cached input tokens written to the provider cache.
  final int cacheWriteTokens = 0,

  /// Recorded model responses, including those before context compaction.
  final List<AssistantMessageItem> requests = const [],
}) {
  /// Creates a usage sample.
  this;
}

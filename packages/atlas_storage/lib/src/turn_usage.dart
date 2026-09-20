import 'package:atlas_runtime/atlas_runtime.dart';

/// One persisted turn's token usage, used for cache reporting.
final class TurnUsageSample {
  /// Creates a usage sample.
  const TurnUsageSample({
    required this.sessionId,
    required this.turnId,
    required this.title,
    required this.startedAt,
    this.providerId,
    this.modelId,
    this.inputTokens = 0,
    this.outputTokens = 0,
    this.cacheReadTokens = 0,
    this.cacheWriteTokens = 0,
    this.requests = const [],
  });

  /// Serialized identifier of the owning session.
  final String sessionId;

  /// Serialized identifier of the turn.
  final String turnId;

  /// Session title at read time.
  final String title;

  /// UTC start time of the turn.
  final DateTime startedAt;

  /// Provider that served the turn, when recorded.
  final String? providerId;

  /// Model that served the turn, when recorded.
  final String? modelId;

  /// Recorded model responses, including those before context compaction.
  final List<AssistantMessageItem> requests;

  /// Input tokens as reported by the provider.
  ///
  /// Anthropic reports this figure without the cached tokens, while
  /// OpenAI-compatible endpoints already include them.
  final int inputTokens;

  /// Output tokens as reported by the provider.
  final int outputTokens;

  /// Cached input tokens read from the provider cache.
  final int cacheReadTokens;

  /// Cached input tokens written to the provider cache.
  final int cacheWriteTokens;
}

/// Token usage returned by a model provider.
final class TokenUsage {
  /// Creates a token usage value.
  const TokenUsage({
    this.inputTokens = 0,
    this.outputTokens = 0,
    this.totalTokens = 0,
    this.cacheReadInputTokens = 0,
    this.cacheWriteInputTokens = 0,
  });

  /// Number of input tokens.
  final int inputTokens;

  /// Number of output tokens.
  final int outputTokens;

  /// Total billed tokens.
  final int totalTokens;

  /// Number of input tokens read from provider cache.
  final int cacheReadInputTokens;

  /// Number of input tokens written to provider cache.
  final int cacheWriteInputTokens;
}

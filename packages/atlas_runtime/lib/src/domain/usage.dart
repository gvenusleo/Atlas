/// Token usage returned by a model provider.
final class TokenUsage {
  /// Creates a token usage value.
  const TokenUsage({
    this.inputTokens = 0,
    this.outputTokens = 0,
    this.totalTokens = 0,
    this.cacheReadInputTokens = 0,
    this.cacheWriteInputTokens = 0,
    this.promptTokens,
    this.cacheReadReported = false,
    this.cacheWriteReported = false,
  });

  /// Number of input tokens in the provider's original accounting convention.
  final int inputTokens;

  /// Number of output tokens.
  final int outputTokens;

  /// Total billed tokens.
  final int totalTokens;

  /// Number of input tokens read from provider cache.
  final int cacheReadInputTokens;

  /// Number of input tokens written to provider cache.
  final int cacheWriteInputTokens;

  /// Complete input size, including cache reads and writes, or unknown.
  ///
  /// Provider adapters normalize this value at response time. Legacy records
  /// and nonstandard usage whose accounting cannot be determined leave it null.
  final int? promptTokens;

  /// Whether the provider explicitly reported the cache-read count.
  final bool cacheReadReported;

  /// Whether the provider explicitly reported the cache-write count.
  final bool cacheWriteReported;

  /// The best-known context occupancy figure: input tokens when reported,
  /// else total tokens (input plus output).
  int get contextTokens => inputTokens > 0 ? inputTokens : totalTokens;
}

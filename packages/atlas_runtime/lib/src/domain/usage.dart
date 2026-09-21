/// Token usage returned by a model provider.
final class const TokenUsage({
  /// Number of input tokens in the provider's original accounting convention.
  final int inputTokens = 0,

  /// Number of output tokens.
  final int outputTokens = 0,

  /// Total billed tokens.
  final int totalTokens = 0,

  /// Number of input tokens read from provider cache.
  final int cacheReadInputTokens = 0,

  /// Number of input tokens written to provider cache.
  final int cacheWriteInputTokens = 0,

  /// Complete input size, including cache reads and writes, or unknown.
  ///
  /// Provider adapters normalize this value at response time. Legacy records
  /// and nonstandard usage whose accounting cannot be determined leave it null.
  final int? promptTokens,

  /// Whether the provider explicitly reported the cache-read count.
  final bool cacheReadReported = false,

  /// Whether the provider explicitly reported the cache-write count.
  final bool cacheWriteReported = false,
}) {
  /// Creates a token usage value.
  this;

  /// The best-known context occupancy figure: input tokens when reported,
  /// else total tokens (input plus output).
  int get contextTokens => inputTokens > 0 ? inputTokens : totalTokens;
}

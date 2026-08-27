/// Atlas-specific ACP extension method names.
const atlasSessionSetTitleMethod = '_atlas.dev/session/set_title';

/// Structured Atlas context compaction extension.
const atlasSessionCompactMethod = '_atlas.dev/session/compact';

/// Structured result returned by [atlasSessionCompactMethod].
final class AtlasCompactResult {
  /// Creates a compaction result.
  const AtlasCompactResult({
    required this.keptMessages,
    required this.tokensBefore,
    required this.tokensAfter,
    required this.summaryPresent,
  });

  /// Decodes an extension response.
  factory AtlasCompactResult.fromJson(Map<String, Object?> json) =>
      AtlasCompactResult(
        keptMessages: (json['keptMessages'] as num?)?.toInt() ?? 0,
        tokensBefore: (json['tokensBefore'] as num?)?.toInt() ?? 0,
        tokensAfter: (json['tokensAfter'] as num?)?.toInt() ?? 0,
        summaryPresent: json['summaryPresent'] == true,
      );

  /// Number of recent timeline messages retained verbatim.
  final int keptMessages;

  /// Estimated input tokens before compaction.
  final int tokensBefore;

  /// Estimated input tokens after compaction.
  final int tokensAfter;

  /// Whether the resulting checkpoint contains a summary.
  final bool summaryPresent;
}

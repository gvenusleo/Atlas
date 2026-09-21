/// Atlas-specific ACP extension method names.
const atlasSessionSetTitleMethod = '_atlas.dev/session/set_title';

/// Structured Atlas context compaction extension.
const atlasSessionCompactMethod = '_atlas.dev/session/compact';

/// Structured result returned by [atlasSessionCompactMethod].
final class const AtlasCompactResult({
  /// Number of recent timeline messages retained verbatim.
  required final int keptMessages,

  /// Estimated input tokens before compaction.
  required final int tokensBefore,

  /// Estimated input tokens after compaction.
  required final int tokensAfter,

  /// Whether the resulting checkpoint contains a summary.
  required final bool summaryPresent,
}) {
  /// Creates a compaction result.
  this;

  /// Decodes an extension response.
  factory fromJson(Map<String, Object?> json) => AtlasCompactResult(
    keptMessages: (json['keptMessages'] as num?)?.toInt() ?? 0,
    tokensBefore: (json['tokensBefore'] as num?)?.toInt() ?? 0,
    tokensAfter: (json['tokensAfter'] as num?)?.toInt() ?? 0,
    summaryPresent: json['summaryPresent'] == true,
  );
}

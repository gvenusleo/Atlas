import 'package:nocterm/nocterm.dart';

/// Session status line below the input bar.
///
/// Mirrors the classic Go TUI footer: the active model, its reasoning effort,
/// and the context usage of the current session.
final class SessionStatusLine extends StatelessComponent {
  /// Creates a status line.
  const SessionStatusLine({
    super.key,
    required this.modelName,
    required this.contextTokens,
    required this.contextWindow,
    this.effortName,
  });

  /// The display name of the active model.
  final String modelName;

  /// The display name of the active reasoning effort, omitted when null.
  final String? effortName;

  /// The total tokens of the most recently finished turn.
  final int contextTokens;

  /// The model context window in tokens.
  final int contextWindow;

  @override
  Component build(BuildContext context) {
    final theme = TuiTheme.of(context);
    final spans = <TextSpan>[
      TextSpan(
        text: '  ${modelName.trim()}',
        style: TextStyle(color: theme.primary),
      ),
    ];
    if (effortName != null && effortName!.isNotEmpty) {
      spans.add(
        TextSpan(
          text: ' ${effortName!.trim()}',
          style: TextStyle(color: theme.warning),
        ),
      );
    }
    spans.add(
      TextSpan(
        text: ' · ',
        style: TextStyle(color: theme.outline),
      ),
    );
    spans.add(
      TextSpan(
        text:
            'Context ${contextUsagePercent(contextTokens, contextWindow)}% used',
        style: TextStyle(color: theme.success),
      ),
    );
    // A single line, clipped at the viewport width like the Go footer.
    return RichText(
      text: TextSpan(children: spans),
      softWrap: false,
      maxLines: 1,
      overflow: TextOverflow.clip,
    );
  }
}

/// Returns the context usage percentage, clamped to 100.
///
/// A missing window or zero tokens reports 0%.
int contextUsagePercent(int tokens, int contextWindow) {
  if (tokens <= 0 || contextWindow <= 0) {
    return 0;
  }
  final percent = tokens * 100 ~/ contextWindow;
  return percent > 100 ? 100 : percent;
}

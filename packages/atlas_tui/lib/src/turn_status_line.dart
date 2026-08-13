import 'package:nocterm/nocterm.dart';

import 'chat_controller.dart';

/// The MiniDot spinner frame sequence, matching the classic Go TUI.
const miniDotFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];

/// Transient status row above the input bar.
///
/// Mirrors the Go TUI turn status: a spinner, the activity label (Working,
/// Thinking, or Compacting), the wall-clock elapsed time, and an
/// esc-to-interrupt hint. Compaction omits the hint because it cannot be
/// interrupted. It renders nothing while no turn is running.
final class TurnStatusLine extends StatelessComponent {
  /// Creates a turn status line.
  const TurnStatusLine({
    super.key,
    required this.phase,
    required this.elapsed,
    required this.frame,
  });

  /// The activity phase of the running turn.
  final TurnPhase phase;

  /// The wall-clock duration since the turn started.
  final Duration elapsed;

  /// The spinner frame counter, advanced by the controller's status tick.
  final int frame;

  @override
  Component build(BuildContext context) {
    if (phase == TurnPhase.idle) {
      return const SizedBox.shrink();
    }
    final theme = TuiTheme.of(context);
    final thinking = phase == TurnPhase.thinking;
    final compacting = phase == TurnPhase.compacting;
    final label = thinking
        ? 'Thinking'
        : compacting
        ? 'Compacting'
        : 'Working';
    final color = thinking ? theme.warning : theme.primary;
    final spinner = miniDotFrames[frame % miniDotFrames.length];
    final hint = compacting
        ? ''
        : ' (${formatTurnElapsed(elapsed)} • esc to interrupt)';
    return RichText(
      text: TextSpan(
        children: [
          TextSpan(
            text: '$spinner $label',
            style: TextStyle(color: color),
          ),
          TextSpan(
            text: hint,
            style: TextStyle(color: theme.outline),
          ),
        ],
      ),
      softWrap: false,
      maxLines: 1,
      overflow: TextOverflow.clip,
    );
  }
}

/// Formats [elapsed] as `12s`, `1m 05s`, or `1h 02m 03s`.
String formatTurnElapsed(Duration elapsed) {
  final seconds = elapsed.inSeconds;
  if (seconds < 60) {
    return '${seconds}s';
  }
  if (seconds < 3600) {
    return '${seconds ~/ 60}m ${(seconds % 60).toString().padLeft(2, '0')}s';
  }
  return '${seconds ~/ 3600}h ${((seconds % 3600) ~/ 60).toString().padLeft(2, '0')}m '
      '${(seconds % 60).toString().padLeft(2, '0')}s';
}

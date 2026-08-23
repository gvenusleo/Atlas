import 'dart:async';
import 'dart:math' as math;

import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../application/workspace_state.dart';

/// Transient working indicator shown above the conversation input.
///
/// Shows three small bouncing dots, the activity label (Working, Thinking, or
/// Compacting), and the wall-clock elapsed time while a turn is active.
class TurnStatusBanner extends StatefulWidget {
  /// Creates a turn status banner.
  const TurnStatusBanner({
    super.key,
    required this.phase,
    required this.startedAt,
  });

  /// Activity phase of the active turn.
  final TurnPhase phase;

  /// When the turn started, or null when it is unknown.
  final DateTime? startedAt;

  @override
  State<TurnStatusBanner> createState() => _TurnStatusBannerState();
}

class _TurnStatusBannerState extends State<TurnStatusBanner>
    with SingleTickerProviderStateMixin {
  late final AnimationController _dots;
  Timer? _elapsedTick;

  @override
  void initState() {
    super.initState();
    _dots = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..repeat();
    _startElapsedTick();
  }

  @override
  void didUpdateWidget(covariant TurnStatusBanner oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.startedAt != null && oldWidget.startedAt == null) {
      _startElapsedTick();
    } else if (widget.startedAt == null && oldWidget.startedAt != null) {
      _elapsedTick?.cancel();
      _elapsedTick = null;
    }
  }

  void _startElapsedTick() {
    if (widget.startedAt == null || _elapsedTick != null) {
      return;
    }
    _elapsedTick = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) {
        setState(() {});
      }
    });
  }

  @override
  void dispose() {
    _elapsedTick?.cancel();
    _dots.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final label = switch (widget.phase) {
      TurnPhase.thinking => 'Thinking',
      TurnPhase.compacting => 'Compacting',
      _ => 'Working',
    };
    final style = TextStyle(color: colors.textSecondary, fontSize: 12.5);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        AnimatedBuilder(
          animation: _dots,
          builder: (context, _) =>
              _TypingDots(value: _dots.value, color: colors.textSecondary),
        ),
        const SizedBox(width: 8),
        Text.rich(
          TextSpan(
            children: [
              TextSpan(text: label, style: style),
              if (widget.startedAt != null)
                TextSpan(
                  text:
                      ' • ${formatTurnDuration(DateTime.now().difference(widget.startedAt!))}',
                  style: style,
                ),
            ],
          ),
        ),
      ],
    );
  }
}

/// Three small dots that bounce in a staggered wave.
class _TypingDots extends StatelessWidget {
  const _TypingDots({required this.value, required this.color});

  /// Progress of the cycle, 0 to 1 and wrapping.
  final double value;

  /// Dot fill color.
  final Color color;

  @override
  Widget build(BuildContext context) {
    // The dots are decorative; keep them out of the semantics tree so the
    // animation does not churn the accessibility bridge.
    return ExcludeSemantics(
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (var i = 0; i < 3; i++)
            Padding(
              padding: EdgeInsets.only(right: i == 2 ? 0 : 2),
              child: Transform.translate(
                offset: Offset(
                  0,
                  -3 * math.max(0, math.sin(2 * math.pi * (value - i * 0.18))),
                ),
                child: Container(
                  key: ValueKey('typing-dot-$i'),
                  width: 3,
                  height: 3,
                  decoration: BoxDecoration(
                    color: color,
                    shape: BoxShape.circle,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}

/// Formats [elapsed] as `12s`, `1m 05s`, or `1h 02m 03s`.
String formatTurnDuration(Duration elapsed) {
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

import 'dart:math' as math;

import 'package:material_ui/material_ui.dart';

import '../../../../../shared/theme/atlas_theme.dart';

/// Formats a token count for compact hover labels such as `128k`.
String compactTokenCount(int tokens) {
  final value = tokens < 0 ? 0 : tokens;
  if (value < 1000) {
    return '$value';
  }
  if (value % 1000 == 0) {
    return '${value ~/ 1000}k';
  }
  final tenths = (value / 100).round() / 10;
  if (tenths == tenths.truncateToDouble()) {
    return '${tenths.toInt()}k';
  }
  return '${tenths}k';
}

/// Formats context usage as `10% · 10k/100k`.
String contextUsageLabel(int usedTokens, int contextWindow) {
  final used = compactTokenCount(usedTokens);
  if (contextWindow <= 0) {
    return used;
  }
  final percent = usedTokens <= 0
      ? 0
      : (usedTokens * 100 / contextWindow).clamp(0, 100).round();
  return '$percent% · $used/${compactTokenCount(contextWindow)}';
}

/// Quiet context-usage ring shown beside the send control.
class const ContextUsageRing({
  super.key,
  required final int usedTokens,
  required final int contextWindow,
}) extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    final progress = contextWindow <= 0
        ? 0.0
        : (usedTokens / contextWindow).clamp(0.0, 1.0);
    final fill = progress >= 0.95
        ? colors.error
        : progress >= 0.8
        ? colors.accent
        : colors.textSecondary;
    final label = contextUsageLabel(usedTokens, contextWindow);
    return Tooltip(
      message: label,
      child: SizedBox.square(
        key: const ValueKey('atlas-context-usage'),
        dimension: 18,
        child: CustomPaint(
          painter: ContextUsageRingPainter(
            progress: progress,
            trackColor: colors.divider,
            fillColor: fill,
          ),
        ),
      ),
    );
  }
}

/// Paints the context-usage ring track and filled arc.
class ContextUsageRingPainter({
  required final double progress,
  required final Color trackColor,
  required final Color fillColor,
}) extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    const stroke = 2.0;
    final radius = (math.min(size.width, size.height) - stroke) / 2;
    final track = Paint()
      ..color = trackColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke;
    canvas.drawCircle(center, radius, track);
    if (progress <= 0) {
      return;
    }
    final fill = Paint()
      ..color = fillColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round;
    canvas.drawArc(
      Rect.fromCircle(center: center, radius: radius),
      -math.pi / 2,
      2 * math.pi * progress,
      false,
      fill,
    );
  }

  @override
  bool shouldRepaint(covariant ContextUsageRingPainter oldDelegate) {
    return oldDelegate.progress != progress ||
        oldDelegate.trackColor != trackColor ||
        oldDelegate.fillColor != fillColor;
  }
}

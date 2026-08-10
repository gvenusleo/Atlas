import 'package:nocterm/nocterm.dart';

/// A user-facing line with the shared prompt styling.
///
/// Used by the input bar and by user messages so both share the same visual
/// language: a subtly gray-shaded background, a `›` prefix, and content.
final class PromptLine extends StatelessComponent {
  /// Creates a prompt line.
  const PromptLine({super.key, required this.child});

  /// The line content.
  final Component child;

  @override
  Component build(BuildContext context) {
    return Container(
      color: _withGrayOverlay(TuiTheme.of(context).background),
      padding: const EdgeInsets.symmetric(vertical: 1),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('› '),
          Expanded(child: child),
        ],
      ),
    );
  }
}

/// Blends a light gray overlay over [color] to shade prompt surfaces.
Color _withGrayOverlay(Color color, [double opacity = 0.1]) {
  return Color.alphaBlend(
    Color.fromARGB((255 * opacity).round(), 128, 128, 128),
    color,
  );
}

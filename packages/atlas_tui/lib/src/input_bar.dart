import 'package:nocterm/nocterm.dart';

/// Bottom bar with the message input and turn status.
final class InputBar extends StatelessComponent {
  /// Creates the input bar.
  const InputBar({
    super.key,
    required this.controller,
    required this.busy,
    required this.onSubmitted,
  });

  /// The text editing state.
  final TextEditingController controller;

  /// Whether a turn is currently running.
  final bool busy;

  /// Called with the submitted text.
  final void Function(String text) onSubmitted;

  @override
  Component build(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: TextField(
            controller: controller,
            focused: true,
            placeholder: busy ? 'Working…' : 'Message Atlas (Enter to send)',
            onSubmitted: onSubmitted,
          ),
        ),
        Text(
          busy ? '● busy' : 'ready',
          style: TextStyle(
            color: busy
                ? Color.fromRGB(240, 180, 40)
                : Color.fromRGB(60, 160, 90),
          ),
        ),
      ],
    );
  }
}

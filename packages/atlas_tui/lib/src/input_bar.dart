import 'package:nocterm/nocterm.dart';

import 'prompt_line.dart';

/// Bottom bar with the message input.
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
    return PromptLine(
      child: TextField(
        controller: controller,
        enabled: !busy,
        focused: true,
        placeholder: busy ? 'Working…' : 'Message Atlas (Enter to send)',
        onSubmitted: onSubmitted,
        maxLines: 10,
        cursorStyle: CursorStyle.underline,
        cursorColor: TuiTheme.of(context).primary,
      ),
    );
  }
}

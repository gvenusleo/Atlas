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
    this.onChanged,
    this.onKeyEvent,
    this.readOnly = false,
  });

  /// The text editing state.
  final TextEditingController controller;

  /// Whether a turn is currently running.
  final bool busy;

  /// Called with the submitted text.
  final void Function(String text) onSubmitted;

  /// Called whenever the input text changes.
  final void Function(String text)? onChanged;

  /// Intercepts key events before the field handles them; return `true` to
  /// consume the event.
  final bool Function(KeyboardEvent event)? onKeyEvent;

  /// Whether the field accepts no text input (model picker mode).
  final bool readOnly;

  @override
  Component build(BuildContext context) {
    return PromptLine(
      child: TextField(
        controller: controller,
        enabled: !busy,
        focused: true,
        readOnly: readOnly,
        placeholder: busy ? 'Working…' : 'Message Atlas (Enter to send)',
        onSubmitted: onSubmitted,
        onChanged: onChanged,
        onKeyEvent: onKeyEvent,
        maxLines: 10,
        cursorStyle: CursorStyle.underline,
        cursorColor: TuiTheme.of(context).primary,
        showCursor: false,
      ),
    );
  }
}

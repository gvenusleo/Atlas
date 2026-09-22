import 'package:nocterm/nocterm.dart';

/// Applies terminal brightness without replacing the mounted application tree.
///
/// NoctermApp adds its theme wrappers only after automatic detection completes,
/// which remounts its child and discards early input. Supplying a preset from
/// the first frame keeps those wrappers present throughout detection.
class const TerminalTheme({
  /// The single terminal brightness query started by application bootstrap.
  required final Future<Brightness> brightness,

  /// The application whose state must survive completion of the query.
  required final Component child,
}) extends StatefulComponent {
  /// Creates a stable theme host with an immediate dark preset.
  this;

  @override
  State<TerminalTheme> createState() => _TerminalThemeState();
}

class _TerminalThemeState extends State<TerminalTheme> {
  TuiThemeData _theme = TuiThemeData.dark;

  @override
  void initState() {
    super.initState();
    component.brightness.then((brightness) {
      if (!mounted) return;
      setState(() {
        _theme = brightness == Brightness.light
            ? TuiThemeData.light
            : TuiThemeData.dark;
      });
    }, onError: (Object _) {});
  }

  @override
  Component build(BuildContext context) =>
      NoctermApp(theme: _theme, child: component.child);
}

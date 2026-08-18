import 'dart:async';

import 'package:material_ui/material_ui.dart';
import 'package:terminal_view/terminal_view.dart';

import '../../../../shared/theme/atlas_theme.dart';
import '../../data/terminal_session.dart';
import '../workspace_metrics.dart';

/// Interactive shell backed by a pseudo-terminal and a terminal emulator.
class TerminalPanel extends StatefulWidget {
  /// Creates a shell rooted at [workingDirectory].
  const TerminalPanel({super.key, required this.workingDirectory});

  /// Initial directory for the shell process.
  final String workingDirectory;

  @override
  State<TerminalPanel> createState() => _TerminalPanelState();
}

class _TerminalPanelState extends State<TerminalPanel> {
  final _terminal = Terminal();
  final _session = TerminalSession();
  bool _starting = false;

  @override
  void initState() {
    super.initState();
    _terminal.onOutput = _writeToPty;
    _terminal.onResize = _resizePty;
    unawaited(_startShell());
  }

  @override
  void didUpdateWidget(covariant TerminalPanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.workingDirectory != widget.workingDirectory) {
      unawaited(_restartShell());
    }
  }

  @override
  void dispose() {
    unawaited(_session.close());
    super.dispose();
  }

  /// Forwards user input from the emulator to the shell.
  void _writeToPty(String data) {
    _session.write(data);
  }

  /// Keeps the pseudo-terminal window size in sync with the emulator.
  void _resizePty(int cols, int rows, int pixelWidth, int pixelHeight) {
    _session.resize(cols, rows);
  }

  Future<void> _startShell() async {
    if (_starting) {
      return;
    }
    _starting = true;
    try {
      await _session.start(
        workingDirectory: widget.workingDirectory,
        onOutput: _terminal.write,
        onExit: (code) {
          if (mounted) {
            _terminal.write('\r\n[Process exited with code $code]\r\n');
            setState(() {});
          }
        },
      );
    } catch (error) {
      _terminal.write('Cannot start shell: $error\r\n');
    } finally {
      _starting = false;
    }
  }

  Future<void> _restartShell() async {
    await _session.close();
    _terminal.write(
      '\r\nWorking directory changed to ${widget.workingDirectory}.\r\n',
    );
    await _startShell();
  }

  @override
  Widget build(BuildContext context) {
    return TerminalView(
      _terminal,
      autofocus: true,
      cursorType: TerminalCursorType.verticalBar,
      padding: const EdgeInsets.only(bottom: 8),
      theme: _terminalTheme(context),
      textStyle: TerminalStyle(
        fontSize: 13,
        fontFamily: WorkspaceMetrics.monospaceFontFamily,
        fontFamilyFallback: const ['SF Mono', 'Monaco', 'monospace'],
      ),
    );
  }
}

/// Builds a terminal palette that follows the active Atlas theme.
TerminalTheme _terminalTheme(BuildContext context) {
  final colors = AtlasColors.of(context);
  final dark = Theme.of(context).brightness == Brightness.dark;
  return TerminalTheme(
    cursor: colors.accent,
    selection: colors.accent.withValues(alpha: 0.35),
    foreground: colors.textPrimary,
    background: colors.canvas,
    black: dark ? const Color(0xFF0D1016) : const Color(0xFF5C6166),
    red: colors.error,
    green: colors.success,
    yellow: dark ? const Color(0xFFFEB454) : const Color(0xFFF1AD49),
    blue: colors.accent,
    magenta: dark ? const Color(0xFF39BAE5) : const Color(0xFF55B4D3),
    cyan: dark ? const Color(0xFF95E5CB) : const Color(0xFF4DBF99),
    white: colors.textPrimary,
    brightBlack: dark ? const Color(0xFF545557) : const Color(0xFF3B9EE5),
    brightRed: dark ? const Color(0xFF83353B) : const Color(0xFFFEBAB6),
    brightGreen: dark ? const Color(0xFF567627) : const Color(0xFFC7D98F),
    brightYellow: dark ? const Color(0xFF92582B) : const Color(0xFFFED5A3),
    brightBlue: dark ? const Color(0xFF27618C) : const Color(0xFFABCDF2),
    brightMagenta: dark ? const Color(0xFF205A78) : const Color(0xFFB1D8E8),
    brightCyan: dark ? const Color(0xFF4C806F) : const Color(0xFFACE0CB),
    brightWhite: dark ? const Color(0xFFFAFAFA) : const Color(0xFFFFFFFF),
    searchHitBackground: colors.raised,
    searchHitBackgroundCurrent: colors.accent,
    searchHitForeground: colors.canvas,
  );
}

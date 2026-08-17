import 'dart:async';
import 'dart:io';

import 'package:material_ui/material_ui.dart';
import 'package:pty2/pty2.dart';
import 'package:terminal_view/terminal_view.dart';

import '../../../../shared/theme/atlas_theme.dart';
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
  PseudoTerminal? _pty;
  StreamSubscription<String>? _outputSubscription;
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
    _outputSubscription?.cancel();
    _pty?.kill();
    super.dispose();
  }

  /// Forwards user input from the emulator to the shell.
  void _writeToPty(String data) {
    _pty?.write(data);
  }

  /// Keeps the pseudo-terminal window size in sync with the emulator.
  void _resizePty(int cols, int rows, int pixelWidth, int pixelHeight) {
    _pty?.resize(cols, rows);
  }

  Future<void> _startShell() async {
    if (_starting) {
      return;
    }
    _starting = true;
    final executable = Platform.isWindows
        ? 'cmd.exe'
        : Platform.environment['SHELL'] ?? '/bin/sh';
    try {
      final pty = PseudoTerminal.start(
        executable,
        const [],
        workingDirectory: widget.workingDirectory,
        environment: const {'TERM': 'xterm-256color'},
      );
      _pty = pty;
      _outputSubscription = pty.out.listen(_terminal.write);
      unawaited(
        pty.exitCode.then((code) {
          if (mounted && identical(_pty, pty)) {
            _terminal.write('\r\n[Process exited with code $code]\r\n');
            setState(() => _pty = null);
          }
        }),
      );
    } catch (error) {
      _terminal.write('Cannot start shell: $error\r\n');
    } finally {
      _starting = false;
    }
  }

  Future<void> _restartShell() async {
    await _outputSubscription?.cancel();
    _pty?.kill();
    _pty = null;
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
    background: colors.panel,
    black: dark ? const Color(0xFF3A404B) : const Color(0xFF4A5568),
    red: colors.error,
    green: colors.success,
    yellow: dark ? const Color(0xFFE5C07B) : const Color(0xFFB7791F),
    blue: colors.accent,
    magenta: dark ? const Color(0xFFC678DD) : const Color(0xFF8E44AD),
    cyan: dark ? const Color(0xFF56B6C2) : const Color(0xFF0E7490),
    white: colors.textPrimary,
    brightBlack: colors.textSecondary,
    brightRed: colors.error,
    brightGreen: colors.success,
    brightYellow: dark ? const Color(0xFFF5D08A) : const Color(0xFFD69E2E),
    brightBlue: colors.accent,
    brightMagenta: dark ? const Color(0xFFD7A0E8) : const Color(0xFFA55EEA),
    brightCyan: dark ? const Color(0xFF6CC7D2) : const Color(0xFF0E9BB8),
    brightWhite: colors.textPrimary,
    searchHitBackground: colors.raised,
    searchHitBackgroundCurrent: colors.accent,
    searchHitForeground: colors.canvas,
  );
}

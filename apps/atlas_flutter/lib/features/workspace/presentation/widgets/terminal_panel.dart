import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:terminal_view/terminal_view.dart';

import '../../application/terminal_registry.dart';
import '../../application/workspace_controller.dart';
import '../../data/terminal_session.dart';
import '../../../../shared/theme/atlas_theme.dart';
import '../workspace_metrics.dart';

/// Keeps one [TerminalPanel] per session so the shell survives focus changes.
class TerminalHost extends ConsumerStatefulWidget {
  /// Creates a host for the focused session's terminal.
  const TerminalHost({
    super.key,
    required this.sessionKey,
    required this.workingDirectory,
  });

  /// Cache key of the focused session or draft.
  final String sessionKey;

  /// Working directory of the focused session.
  final String workingDirectory;

  @override
  ConsumerState<TerminalHost> createState() => _TerminalHostState();
}

class _TerminalHostState extends ConsumerState<TerminalHost> {
  final _directories = <String, String>{};
  final _panelKeys = <String, GlobalKey>{};

  @override
  Widget build(BuildContext context) {
    final liveKeys = ref.watch(
      workspaceProvider.select((state) => state.workspaces.keys.toSet()),
    );
    final directories = <String, String>{
      for (final entry in _directories.entries)
        if (liveKeys.contains(entry.key)) entry.key: entry.value,
      widget.sessionKey: widget.workingDirectory,
    };
    _directories
      ..clear()
      ..addAll(directories);
    _panelKeys.removeWhere((key, _) => !directories.containsKey(key));
    final keys = directories.keys.toList();
    return IndexedStack(
      index: keys.indexOf(widget.sessionKey),
      children: [
        for (final key in keys)
          TerminalPanel(
            key: _panelKeys.putIfAbsent(key, GlobalKey.new),
            workingDirectory: directories[key]!,
          ),
      ],
    );
  }
}

/// Interactive shell backed by a pseudo-terminal and a terminal emulator.
class TerminalPanel extends ConsumerStatefulWidget {
  /// Creates a shell rooted at [workingDirectory].
  const TerminalPanel({super.key, required this.workingDirectory});

  /// Initial directory for the shell process.
  final String workingDirectory;

  @override
  ConsumerState<TerminalPanel> createState() => _TerminalPanelState();
}

class _TerminalPanelState extends ConsumerState<TerminalPanel> {
  final _terminal = Terminal();
  final _session = TerminalSession();
  late final TerminalSessionRegistry _registry;
  bool _starting = false;

  @override
  void initState() {
    super.initState();
    // The registry kills this shell when the application exits: disposing the
    // panel is not part of the window-close path. It is captured here because
    // `ref` cannot be read from `dispose`.
    _registry = ref.read(terminalSessionRegistryProvider);
    _registry.track(_session);
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
    _registry.untrack(_session);
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
      theme: _terminalThemeFor(
        AtlasPalette.standard,
        Theme.of(context).brightness,
      ),
      textStyle: TerminalStyle(
        fontSize: 13,
        fontFamily: WorkspaceMetrics.monospaceFontFamily,
        fontFamilyFallback: const ['SF Mono', 'Monaco', 'monospace'],
        height: 1.2,
      ),
    );
  }
}

/// Builds the terminal theme from the palette's terminal colors.
///
/// Bold is not remapped onto the bright slots: bold text keeps the normal
/// ANSI color so `ls` / starship output stays readable.
TerminalTheme _terminalThemeFor(AtlasPalette palette, Brightness brightness) {
  final t = palette.terminal(brightness);
  return TerminalTheme(
    cursor: t.cursor,
    selection: t.selection,
    foreground: t.foreground,
    background: t.background,
    black: t.black,
    red: t.red,
    green: t.green,
    yellow: t.yellow,
    blue: t.blue,
    magenta: t.magenta,
    cyan: t.cyan,
    white: t.white,
    brightBlack: t.brightBlack,
    brightRed: t.brightRed,
    brightGreen: t.brightGreen,
    brightYellow: t.brightYellow,
    brightBlue: t.brightBlue,
    brightMagenta: t.brightMagenta,
    brightCyan: t.brightCyan,
    brightWhite: t.brightWhite,
    searchHitBackground: t.searchHitBackground,
    searchHitBackgroundCurrent: t.searchHitBackgroundCurrent,
    searchHitForeground: t.searchHitForeground,
    drawBoldTextInBrightColors: false,
  );
}

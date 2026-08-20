import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:terminal_view/terminal_view.dart';

import '../../application/workspace_controller.dart';
import '../../data/terminal_session.dart';
import '../workspace_metrics.dart';

/// Maximum number of live shells kept when switching sessions.
const _maxLiveTerminals = 4;

/// Keeps one [TerminalPanel] per recent session and recycles idle shells.
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
  final _order = <String>[];
  final _directories = <String, String>{};

  @override
  Widget build(BuildContext context) {
    final liveKeys = ref.watch(
      workspaceProvider.select((state) => state.workspaces.keys.toSet()),
    );
    final order = [
      for (final key in _order)
        if (liveKeys.contains(key) && key != widget.sessionKey) key,
      widget.sessionKey,
    ];
    while (order.length > _maxLiveTerminals) {
      order.removeAt(0);
    }
    final directories = {
      for (final key in order)
        key: key == widget.sessionKey
            ? widget.workingDirectory
            : _directories[key] ?? widget.workingDirectory,
    };
    _order
      ..clear()
      ..addAll(order);
    _directories
      ..clear()
      ..addAll(directories);
    return IndexedStack(
      index: order.indexOf(widget.sessionKey),
      children: [
        for (final key in order)
          TerminalPanel(
            key: ValueKey('term-$key'),
            workingDirectory: directories[key]!,
          ),
      ],
    );
  }
}

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
      theme: Theme.of(context).brightness == Brightness.dark
          ? _ayuDarkTerminalTheme
          : _ayuLightTerminalTheme,
      textStyle: TerminalStyle(
        fontSize: 13,
        fontFamily: WorkspaceMetrics.monospaceFontFamily,
        fontFamilyFallback: const ['SF Mono', 'Monaco', 'monospace'],
      ),
    );
  }
}

/// Ayu Light ANSI palette from Zed's `ayu.json`.
///
/// Bold is not remapped onto the bright slots: those colors are pastel
/// variants in Ayu, and `ls` / starship use bold + a normal ANSI color.
const _ayuLightTerminalTheme = TerminalTheme(
  cursor: Color(0xFF3B9EE5),
  selection: Color(0x3D3B9EE5),
  foreground: Color(0xFF5C6166),
  background: Color(0xFFFCFCFC),
  black: Color(0xFF5C6166),
  red: Color(0xFFEF7271),
  green: Color(0xFF85B304),
  yellow: Color(0xFFF1AD49),
  blue: Color(0xFF3B9EE5),
  magenta: Color(0xFF55B4D3),
  cyan: Color(0xFF4DBF99),
  white: Color(0xFFFCFCFC),
  brightBlack: Color(0xFF3B9EE5),
  brightRed: Color(0xFFFEBAB6),
  brightGreen: Color(0xFFC7D98F),
  brightYellow: Color(0xFFFED5A3),
  brightBlue: Color(0xFFABCDF2),
  brightMagenta: Color(0xFFB1D8E8),
  brightCyan: Color(0xFFACE0CB),
  brightWhite: Color(0xFFFFFFFF),
  searchHitBackground: Color(0x663B9EE5),
  searchHitBackgroundCurrent: Color(0x66F88B36),
  searchHitForeground: Color(0xFF5C6166),
  drawBoldTextInBrightColors: false,
);

/// Ayu Dark ANSI palette from Zed's `ayu.json`.
const _ayuDarkTerminalTheme = TerminalTheme(
  cursor: Color(0xFF5AC1FE),
  selection: Color(0x3D5AC1FE),
  foreground: Color(0xFFBFBDB6),
  background: Color(0xFF0D1016),
  black: Color(0xFF0D1016),
  red: Color(0xFFEF7177),
  green: Color(0xFFAAD84C),
  yellow: Color(0xFFFEB454),
  blue: Color(0xFF5AC1FE),
  magenta: Color(0xFF39BAE5),
  cyan: Color(0xFF95E5CB),
  white: Color(0xFFBFBDB6),
  brightBlack: Color(0xFF545557),
  brightRed: Color(0xFF83353B),
  brightGreen: Color(0xFF567627),
  brightYellow: Color(0xFF92582B),
  brightBlue: Color(0xFF27618C),
  brightMagenta: Color(0xFF205A78),
  brightCyan: Color(0xFF4C806F),
  brightWhite: Color(0xFFFAFAFA),
  searchHitBackground: Color(0x665AC2FE),
  searchHitBackgroundCurrent: Color(0x66EA5701),
  searchHitForeground: Color(0xFFBFBDB6),
  drawBoldTextInBrightColors: false,
);

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:terminal_view/terminal_view.dart';

import '../../application/terminal_registry.dart';
import '../../application/workspace_controller.dart';
import '../../data/terminal_session.dart';
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
      theme: Theme.of(context).brightness == Brightness.dark
          ? _temperDarkTerminalTheme
          : _temperLightTerminalTheme,
      textStyle: TerminalStyle(
        fontSize: 13,
        fontFamily: WorkspaceMetrics.monospaceFontFamily,
        fontFamilyFallback: const ['SF Mono', 'Monaco', 'monospace'],
        height: 1.2,
      ),
    );
  }
}

/// Token Temper light ANSI palette from `ThorstenRhau/token` @ `1538e1e`.
///
/// Colors come from the upstream Apple Terminal profile
/// (`contrib/apple-terminal/token-temper-light.terminal`): slots 0-15,
/// foreground, background, cursor, and selection are transcribed as exported.
/// Bold is not remapped onto the bright slots: the upstream `TextBoldColor`
/// equals the normal `TextColor`, and `ls` / starship rely on bold with a
/// normal ANSI color.
const _temperLightTerminalTheme = TerminalTheme(
  cursor: Color(0xFF283039),
  selection: Color(0xFFDCE2E7),
  foreground: Color(0xFF283039),
  background: Color(0xFFF5F7F8),
  black: Color(0xFF283039),
  red: Color(0xFFBF3F50),
  green: Color(0xFF005B53),
  yellow: Color(0xFF946B1E),
  blue: Color(0xFF005850),
  magenta: Color(0xFF612C8D),
  cyan: Color(0xFF005B53),
  white: Color(0xFFA7B2BB),
  brightBlack: Color(0xFF414C57),
  brightRed: Color(0xFF005850),
  brightGreen: Color(0xFF004A44),
  brightYellow: Color(0xFF683495),
  brightBlue: Color(0xFF005B53),
  brightMagenta: Color(0xFF7646A2),
  brightCyan: Color(0xFF16746B),
  brightWhite: Color(0xFFF5F7F8),
  searchHitBackground: Color(0xFFE9DDBE),
  searchHitBackgroundCurrent: Color(0xFF005850),
  searchHitForeground: Color(0xFF283039),
  drawBoldTextInBrightColors: false,
);

/// Token Temper dark ANSI palette from `ThorstenRhau/token` @ `1538e1e`.
///
/// Same Apple Terminal export as the light palette. Search hits have no
/// terminal equivalent upstream, so they use the Neovim roles: `Search`
/// (`match`) for matches and `CurSearch` (`accent`) for the active one.
const _temperDarkTerminalTheme = TerminalTheme(
  cursor: Color(0xFFC3C8CC),
  selection: Color(0xFF3A414A),
  foreground: Color(0xFFC3C8CC),
  background: Color(0xFF272C33),
  black: Color(0xFF1C2127),
  red: Color(0xFFE3888C),
  green: Color(0xFF5EC4B5),
  yellow: Color(0xFFC5A15A),
  blue: Color(0xFF51B8AA),
  magenta: Color(0xFFC294E6),
  cyan: Color(0xFF62C7B9),
  white: Color(0xFFA2A9AF),
  brightBlack: Color(0xFF858F9B),
  brightRed: Color(0xFF5CC1B3),
  brightGreen: Color(0xFF76D5C7),
  brightYellow: Color(0xFFE7CDFF),
  brightBlue: Color(0xFF64C3B6),
  brightMagenta: Color(0xFFC69AE7),
  brightCyan: Color(0xFF72D1C3),
  brightWhite: Color(0xFFC3C8CC),
  searchHitBackground: Color(0xFF4A402E),
  searchHitBackgroundCurrent: Color(0xFF5CC1B3),
  searchHitForeground: Color(0xFFC3C8CC),
  drawBoldTextInBrightColors: false,
);

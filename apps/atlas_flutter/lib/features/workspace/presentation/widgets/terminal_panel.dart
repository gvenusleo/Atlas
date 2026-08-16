import 'dart:async';
import 'dart:io';

import 'package:cupertino_ui/cupertino_ui.dart';
import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../../../../shared/theme/atlas_theme.dart';

const _maximumTerminalCharacters = 100000;

/// Persistent local shell with streamed output and command history.
class TerminalPanel extends StatefulWidget {
  /// Creates a shell rooted at [workingDirectory].
  const TerminalPanel({super.key, required this.workingDirectory});

  /// Initial directory for the shell process.
  final String workingDirectory;

  @override
  State<TerminalPanel> createState() => _TerminalPanelState();
}

class _TerminalPanelState extends State<TerminalPanel> {
  final _inputController = TextEditingController();
  final _scrollController = ScrollController();
  final _output = StringBuffer();
  final _history = <String>[];
  Process? _process;
  StreamSubscription<String>? _stdoutSubscription;
  StreamSubscription<String>? _stderrSubscription;
  int _historyIndex = 0;
  bool _starting = false;

  @override
  void initState() {
    super.initState();
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
    _stdoutSubscription?.cancel();
    _stderrSubscription?.cancel();
    _process?.kill();
    _inputController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = AtlasColors.of(context);
    return Column(
      children: [
        Expanded(
          child: SelectionArea(
            child: SingleChildScrollView(
              controller: _scrollController,
              padding: const EdgeInsets.fromLTRB(10, 10, 10, 16),
              child: Align(
                alignment: Alignment.topLeft,
                child: Text(
                  _output.toString(),
                  style: TextStyle(
                    color: colors.textPrimary,
                    fontFamily: 'monospace',
                    fontSize: 11.5,
                    height: 1.45,
                  ),
                ),
              ),
            ),
          ),
        ),
        const Divider(),
        Padding(
          padding: const EdgeInsets.fromLTRB(10, 6, 6, 8),
          child: Row(
            children: [
              Text(
                '>',
                style: TextStyle(
                  color: colors.accent,
                  fontFamily: 'monospace',
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(width: 7),
              Expanded(
                child: Focus(
                  onKeyEvent: _handleHistoryKey,
                  child: TextField(
                    key: const ValueKey('atlas-terminal-input'),
                    controller: _inputController,
                    enabled: _process != null && !_starting,
                    maxLines: 1,
                    onSubmitted: (_) => _submit(),
                    style: TextStyle(
                      color: colors.textPrimary,
                      fontFamily: 'monospace',
                      fontSize: 12,
                    ),
                    decoration: InputDecoration(
                      border: InputBorder.none,
                      isCollapsed: true,
                      hintText: _starting ? 'Starting shell' : null,
                      hintStyle: TextStyle(color: colors.textSecondary),
                    ),
                  ),
                ),
              ),
              Tooltip(
                message: 'Run command',
                child: CupertinoButton(
                  padding: EdgeInsets.zero,
                  minimumSize: const Size.square(32),
                  pressedOpacity: 0.72,
                  onPressed: _process == null || _starting ? null : _submit,
                  child: Icon(
                    CupertinoIcons.return_icon,
                    size: 15,
                    color: colors.textSecondary,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Future<void> _startShell() async {
    if (_starting) {
      return;
    }
    setState(() => _starting = true);
    final executable = Platform.isWindows
        ? 'cmd.exe'
        : Platform.environment['SHELL'] ?? '/bin/sh';
    try {
      final process = await Process.start(
        executable,
        const [],
        workingDirectory: widget.workingDirectory,
        environment: {...Platform.environment, 'TERM': 'dumb', 'PS1': ''},
      );
      _process = process;
      _append('Shell: $executable\n');
      _stdoutSubscription = process.stdout
          .transform(systemEncoding.decoder)
          .listen(_append);
      _stderrSubscription = process.stderr
          .transform(systemEncoding.decoder)
          .listen(_append);
      unawaited(
        process.exitCode.then((code) {
          if (mounted && identical(_process, process)) {
            _append('\nShell exited with code $code.\n');
            setState(() => _process = null);
          }
        }),
      );
    } on ProcessException catch (error) {
      _append('Cannot start shell: ${error.message}\n');
    } finally {
      if (mounted) {
        setState(() => _starting = false);
      }
    }
  }

  Future<void> _restartShell() async {
    await _stdoutSubscription?.cancel();
    await _stderrSubscription?.cancel();
    _process?.kill();
    _process = null;
    _append('\nWorking directory changed to ${widget.workingDirectory}.\n');
    await _startShell();
  }

  void _submit() {
    final command = _inputController.text.trimRight();
    final process = _process;
    if (command.trim().isEmpty || process == null) {
      return;
    }
    _history.add(command);
    _historyIndex = _history.length;
    _inputController.clear();
    _append('> $command\n');
    process.stdin.writeln(command);
  }

  KeyEventResult _handleHistoryKey(FocusNode node, KeyEvent event) {
    if (event is! KeyDownEvent || _history.isEmpty) {
      return KeyEventResult.ignored;
    }
    if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
      _historyIndex = (_historyIndex - 1).clamp(0, _history.length - 1);
    } else if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
      _historyIndex = (_historyIndex + 1).clamp(0, _history.length);
    } else {
      return KeyEventResult.ignored;
    }
    _inputController.text = _historyIndex == _history.length
        ? ''
        : _history[_historyIndex];
    _inputController.selection = TextSelection.collapsed(
      offset: _inputController.text.length,
    );
    return KeyEventResult.handled;
  }

  void _append(String text) {
    if (!mounted) {
      return;
    }
    final combined = '${_output.toString()}$text';
    _output
      ..clear()
      ..write(
        combined.length <= _maximumTerminalCharacters
            ? combined
            : combined.substring(combined.length - _maximumTerminalCharacters),
      );
    setState(() {});
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _scrollController.hasClients) {
        _scrollController.jumpTo(_scrollController.position.maxScrollExtent);
      }
    });
  }
}

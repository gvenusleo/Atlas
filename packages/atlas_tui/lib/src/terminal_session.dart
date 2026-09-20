import 'dart:async';
import 'dart:io';

import 'package:nocterm/nocterm.dart';

/// Whether stdio can safely host the full-screen ANSI interface.
bool get supportsAtlasTui =>
    stdin.hasTerminal &&
    stdout.hasTerminal &&
    stdout.supportsAnsiEscapes &&
    !Platform.environment.containsKey('NO_COLOR');

/// Runs a Nocterm tree without allowing the framework to exit the process.
///
/// An injected [backend] is owned and disposed by this invocation. Real stdio
/// is checked before any terminal modes or escape sequences are changed.
Future<void> runTerminalSession(
  Component Function(void Function() quit) build, {
  TerminalBackend? backend,
  bool enableHotReload = true,
}) async {
  if (backend == null && !supportsAtlasTui) {
    throw StateError('The TUI requires an ANSI terminal with NO_COLOR unset');
  }
  final terminalBackend = backend ?? _NaturalStdioBackend();
  _AtlasTerminalBinding? binding;
  StreamSubscription<ProcessSignal>? terminate;
  try {
    final active = _AtlasTerminalBinding(Terminal(terminalBackend));
    binding = active;
    if (backend == null && !Platform.isWindows) {
      terminate = ProcessSignal.sigterm.watch().listen(
        (_) => active.requestShutdown(),
      );
    }
    active.initialize();
    active.attachRootComponent(
      DebugOverlay(
        child: build(() {
          active.requestShutdown();
        }),
      ),
    );
    if (enableHotReload && !const bool.fromEnvironment('dart.vm.product')) {
      // Nocterm prints hot-reload diagnostics. Keep them off result stdout.
      await runZoned(
        active.initializeHotReload,
        zoneSpecification: ZoneSpecification(
          print: (_, _, _, message) => stderr.writeln(message),
        ),
      );
    }
    await active.runEventLoop();
  } finally {
    try {
      // The framework's stdio bootstrap neither disposes its backend nor
      // unmounts the tree. Both are necessary for timers/signals to drain.
      binding?.requestShutdown();
      final root = binding?.rootElement;
      if (root != null) _unmount(root);
      binding?.buildOwner.finalizeTree();
    } finally {
      await terminate?.cancel();
      try {
        terminalBackend.disableRawMode();
      } finally {
        terminalBackend.dispose();
        await Future.wait([stdout.flush(), stderr.flush()]);
      }
    }
  }
}

void _unmount(Element element) {
  element.deactivate();
  element.visitChildren(_unmount);
  if (element is RenderObjectElement) element.renderObject.dispose();
  element.unmount();
}

class _AtlasTerminalBinding extends TerminalBinding {
  _AtlasTerminalBinding(super.terminal);

  @override
  void scheduleFrameImpl() {
    if (!shouldExit) super.scheduleFrameImpl();
  }
}

class _NaturalStdioBackend extends StdioBackend {
  StreamSubscription<List<int>>? _inputSubscription;
  late final StreamController<List<int>> _input = StreamController<List<int>>(
    onListen: () {
      _inputSubscription = super.inputStream!.listen(
        (bytes) {
          if (_input.hasListener) _input.add(bytes);
        },
        onError: (Object error, StackTrace stack) {
          if (_input.hasListener) _input.addError(error, stack);
        },
        onDone: () => TerminalBinding.instance.requestShutdown(),
      );
    },
  );

  @override
  Stream<List<int>> get inputStream => _input.stream;

  @override
  void dispose() {
    // Cancelling stdin closes its descriptor. Keep it open until raw mode has
    // been restored, even when Nocterm cancels its own input subscription.
    disableRawMode();
    unawaited(_inputSubscription?.cancel());
    unawaited(_input.close());
    super.dispose();
  }

  bool? _echoMode;
  bool? _lineMode;

  @override
  void enableRawMode() {
    if (!stdin.hasTerminal) return;
    _echoMode ??= stdin.echoMode;
    _lineMode ??= stdin.lineMode;
    super.enableRawMode();
  }

  @override
  void disableRawMode() {
    if (!stdin.hasTerminal) return;
    if (_lineMode case final previous?) stdin.lineMode = previous;
    if (_echoMode case final previous?) stdin.echoMode = previous;
  }

  @override
  void requestExit([int exitCode = 0]) {
    // TerminalBinding already stopped the event loop and restored the screen.
    // The caller owns process status and asynchronous adapter teardown.
    writeRaw(EscapeCodes.disable.bracketedPasteMode);
  }
}

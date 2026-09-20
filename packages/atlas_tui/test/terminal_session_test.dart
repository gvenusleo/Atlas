import 'dart:async';
import 'dart:isolate';

import 'package:atlas_tui/src/terminal_session.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  test(
    'quit restores the screen, disposes the tree, and returns naturally',
    () async {
      final result = await Isolate.run(() async {
        final backend = _Backend();
        var unmounted = false;
        await runTerminalSession(
          (quit) => _Lifecycle(
            onMount: () => scheduleMicrotask(quit),
            onDispose: () => unmounted = true,
          ),
          backend: backend,
          enableHotReload: false,
        );
        return (
          unmounted,
          backend.disposed,
          backend.rawMode,
          backend.output.toString(),
          backend.exitRequests,
        );
      });
      expect(result.$1, isTrue);
      expect(result.$2, isTrue);
      expect(result.$3, isFalse);
      expect(result.$4, contains('\x1b[?25h'));
      expect(result.$5, greaterThan(0));
    },
  );

  test(
    'termination events return rather than killing the test process',
    () async {
      final result = await Isolate.run(() async {
        final backend = _Backend();
        await runTerminalSession(
          (_) {
            scheduleMicrotask(() => backend.signals.add(null));
            return Text('ready');
          },
          backend: backend,
          enableHotReload: false,
        );
        return (backend.disposed, backend.rawMode, backend.exitRequests);
      });
      expect(result.$1, isTrue);
      expect(result.$2, isFalse);
      expect(result.$3, greaterThan(0));
    },
  );

  test(
    'initialization errors still restore modes and dispose the backend',
    () async {
      final result = await Isolate.run(() async {
        final backend = _Backend();
        Object? failure;
        try {
          await runTerminalSession(
            (_) => throw StateError('mount failed'),
            backend: backend,
            enableHotReload: false,
          );
        } catch (error) {
          failure = error;
        }
        return (
          failure,
          backend.disposed,
          backend.rawMode,
          backend.output.toString(),
        );
      });
      expect(result.$1, isA<StateError>());
      expect(result.$2, isTrue);
      expect(result.$3, isFalse);
      expect(result.$4, contains('\x1b[?25h'));
    },
  );
}

class _Backend extends TerminalBackend {
  final input = StreamController<List<int>>();
  final signals = StreamController<void>.broadcast();
  final output = StringBuffer();
  bool rawMode = false;
  bool disposed = false;
  int exitRequests = 0;

  @override
  void writeRaw(String data) => output.write(data);
  @override
  Size getSize() => const Size(80, 24);
  @override
  bool get supportsSize => true;
  @override
  Stream<List<int>> get inputStream => input.stream;
  @override
  Stream<Size>? get resizeStream => null;
  @override
  Stream<void> get shutdownStream => signals.stream;
  @override
  void enableRawMode() => rawMode = true;
  @override
  void disableRawMode() => rawMode = false;
  @override
  bool get isAvailable => !disposed;
  @override
  void requestExit([int exitCode = 0]) => exitRequests++;
  @override
  void dispose() {
    disposed = true;
    unawaited(input.close());
    unawaited(signals.close());
  }
}

class _Lifecycle extends StatefulComponent {
  _Lifecycle({required this.onMount, required this.onDispose});
  final void Function() onMount;
  final void Function() onDispose;
  @override
  State<_Lifecycle> createState() => _LifecycleState();
}

class _LifecycleState extends State<_Lifecycle> {
  @override
  void initState() {
    super.initState();
    component.onMount();
  }

  @override
  Component build(BuildContext context) => Text('ready');
  @override
  void dispose() {
    component.onDispose();
    super.dispose();
  }
}

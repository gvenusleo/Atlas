import 'package:atlas_flutter/features/workspace/application/terminal_registry.dart';
import 'package:atlas_flutter/features/workspace/data/terminal_session.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('tracks and forgets shells', () {
    final registry = TerminalSessionRegistry();
    final first = _RecordingHandle();
    final second = _RecordingHandle();

    registry.track(first);
    registry.track(second);
    expect(registry.length, 2);
    expect(registry.isEmpty, isFalse);

    registry.untrack(first);
    expect(registry.length, 1);

    registry.closeAll();
    expect(second.killCount, 1);
    // An untracked shell is not the registry's responsibility anymore.
    expect(first.killCount, 0);
    expect(registry.isEmpty, isTrue);
  });

  test('closeAll kills every tracked shell exactly once', () {
    final registry = TerminalSessionRegistry();
    final handles = [
      _RecordingHandle(),
      _RecordingHandle(),
      _RecordingHandle(),
    ];
    for (final handle in handles) {
      registry.track(handle);
    }

    registry.closeAll();
    registry.closeAll();

    expect(handles.map((handle) => handle.killCount), [1, 1, 1]);
    expect(registry.isEmpty, isTrue);
  });

  test('closeAll tolerates a shell that throws on kill', () {
    final registry = TerminalSessionRegistry();
    final throwing = _ThrowingHandle();
    final healthy = _RecordingHandle();
    registry.track(throwing);
    registry.track(healthy);

    expect(registry.closeAll, returnsNormally);
    expect(throwing.killCount, 1);
    // A failing shell must not leave the remaining ones alive: the exit path
    // depends on every PTY being released.
    expect(healthy.killCount, 1);
    expect(registry.isEmpty, isTrue);
  });

  test('a never-started session is safe to kill and close', () async {
    final session = TerminalSession();

    expect(session.isRunning, isFalse);
    expect(session.kill, returnsNormally);
    await session.close();
    expect(session.isRunning, isFalse);
  });
}

final class _RecordingHandle implements TerminalHandle {
  int killCount = 0;

  @override
  void kill() => killCount++;
}

final class _ThrowingHandle implements TerminalHandle {
  int killCount = 0;

  @override
  void kill() {
    killCount++;
    throw StateError('kill failed');
  }
}

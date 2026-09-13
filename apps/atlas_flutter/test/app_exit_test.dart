import 'dart:ui' show AppExitResponse;

import 'package:atlas_flutter/app/atlas_app.dart';
import 'package:atlas_flutter/features/workspace/application/terminal_registry.dart';
import 'package:atlas_flutter/features/workspace/data/terminal_session.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('an exit request kills the tracked shells and exits', (
    tester,
  ) async {
    await tester.pumpWidget(const ProviderScope(child: AtlasApp()));
    await tester.pump();
    final container = ProviderScope.containerOf(
      tester.element(find.byType(AtlasApp)),
    );
    final registry = container.read(terminalSessionRegistryProvider);
    final shell = _RecordingHandle();
    registry.track(shell);

    // The embedder asks before it terminates; a live PTY must be released here
    // or the isolate shuts down with a blocking `close` on its master.
    final response = await tester.binding.handleRequestAppExit();

    expect(response, AppExitResponse.exit);
    expect(shell.killCount, 1);
    expect(registry.isEmpty, isTrue);
  });

  testWidgets('a shell that refuses to die still lets the app exit', (
    tester,
  ) async {
    await tester.pumpWidget(const ProviderScope(child: AtlasApp()));
    await tester.pump();
    final container = ProviderScope.containerOf(
      tester.element(find.byType(AtlasApp)),
    );
    final registry = container.read(terminalSessionRegistryProvider);
    registry.track(_ThrowingHandle());
    final healthy = _RecordingHandle();
    registry.track(healthy);

    final response = await tester.binding.handleRequestAppExit();

    expect(response, AppExitResponse.exit);
    expect(healthy.killCount, 1);
    expect(registry.isEmpty, isTrue);
  });
}

final class _RecordingHandle implements TerminalHandle {
  int killCount = 0;

  @override
  void kill() => killCount++;
}

final class _ThrowingHandle implements TerminalHandle {
  @override
  void kill() => throw StateError('kill failed');
}

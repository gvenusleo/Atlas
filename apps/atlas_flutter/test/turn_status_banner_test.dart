import 'package:atlas_flutter/features/workspace/application/workspace_state.dart';
import 'package:atlas_flutter/features/workspace/presentation/widgets/turn_status_banner.dart';
import 'package:atlas_flutter/shared/theme/atlas_theme.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

void main() {
  testWidgets('renders the activity label for each phase', (tester) async {
    for (final (phase, label) in [
      (TurnPhase.working, 'Working'),
      (TurnPhase.thinking, 'Thinking'),
      (TurnPhase.compacting, 'Compacting'),
    ]) {
      await tester.pumpWidget(
        MaterialApp(
          theme: buildAtlasTheme(Brightness.light),
          home: Scaffold(
            body: TurnStatusBanner(phase: phase, startedAt: DateTime.now()),
          ),
        ),
      );
      await tester.pump();
      expect(find.textContaining(label), findsOneWidget);
    }
    // Unmount so the banner's elapsed timer is disposed before the test ends.
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('typing dots bounce over time', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: Scaffold(
          body: TurnStatusBanner(
            phase: TurnPhase.working,
            startedAt: DateTime.now(),
          ),
        ),
      ),
    );
    final before = tester.getTopLeft(
      find.byKey(const ValueKey('typing-dot-0')),
    );
    await tester.pump(const Duration(milliseconds: 150));
    final after = tester.getTopLeft(find.byKey(const ValueKey('typing-dot-0')));
    expect(before, isNot(after));
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('omits the elapsed time when the turn start is unknown', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: buildAtlasTheme(Brightness.light),
        home: Scaffold(
          body: TurnStatusBanner(phase: TurnPhase.working, startedAt: null),
        ),
      ),
    );
    await tester.pump();
    expect(find.textContaining('Working'), findsOneWidget);
    expect(find.textContaining('•'), findsNothing);
    // A pending one-second timer would fail this test; with no start time the
    // banner must not schedule one.
    await tester.pumpWidget(const SizedBox());
  });

  test('formatTurnDuration renders compact elapsed time', () {
    expect(formatTurnDuration(const Duration(seconds: 5)), '5s');
    expect(formatTurnDuration(const Duration(seconds: 65)), '1m 05s');
    expect(formatTurnDuration(const Duration(seconds: 3723)), '1h 02m 03s');
  });
}

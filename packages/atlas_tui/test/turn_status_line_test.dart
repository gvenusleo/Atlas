import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  group('formatTurnElapsed', () {
    test('formats seconds', () {
      expect(formatTurnElapsed(const Duration(seconds: 0)), '0s');
      expect(formatTurnElapsed(const Duration(seconds: 59)), '59s');
    });

    test('formats minutes and seconds', () {
      expect(formatTurnElapsed(const Duration(seconds: 60)), '1m 00s');
      expect(formatTurnElapsed(const Duration(seconds: 65)), '1m 05s');
      expect(formatTurnElapsed(const Duration(seconds: 3599)), '59m 59s');
    });

    test('formats hours, minutes, and seconds', () {
      expect(formatTurnElapsed(const Duration(seconds: 3723)), '1h 02m 03s');
    });
  });

  group('TurnStatusLine', () {
    test('renders the working status with elapsed and esc hint', () async {
      await testNocterm('working status', (tester) async {
        await tester.pumpComponent(
          const TurnStatusLine(
            phase: TurnPhase.working,
            elapsed: Duration(seconds: 12),
            frame: 0,
          ),
        );

        expect(tester.terminalState, containsText('Working'));
        expect(tester.terminalState, containsText('(12s • esc to interrupt)'));
      });
    });

    test('renders the thinking status', () async {
      await testNocterm('thinking status', (tester) async {
        await tester.pumpComponent(
          const TurnStatusLine(
            phase: TurnPhase.thinking,
            elapsed: Duration(minutes: 1, seconds: 5),
            frame: 3,
          ),
        );

        expect(tester.terminalState, containsText('Thinking'));
        expect(
          tester.terminalState,
          containsText('(1m 05s • esc to interrupt)'),
        );
      });
    });

    test('renders nothing when idle', () async {
      await testNocterm('idle status', (tester) async {
        await tester.pumpComponent(
          const TurnStatusLine(
            phase: TurnPhase.idle,
            elapsed: Duration.zero,
            frame: 0,
          ),
        );

        expect(tester.terminalState.getText().trim(), equals(''));
      });
    });
  });
}

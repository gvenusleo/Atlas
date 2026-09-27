import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
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

    test('renders the compacting status without the interrupt hint', () async {
      await testNocterm('compacting status', (tester) async {
        await tester.pumpComponent(
          const TurnStatusLine(
            phase: TurnPhase.compacting,
            elapsed: Duration(seconds: 3),
            frame: 0,
          ),
        );

        expect(tester.terminalState, containsText('Compacting'));
        expect(tester.terminalState, isNot(containsText('esc to interrupt')));
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

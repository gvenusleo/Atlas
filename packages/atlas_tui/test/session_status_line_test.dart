import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  group('contextUsagePercent', () {
    test('clamps to 100', () {
      expect(contextUsagePercent(2000, 1000), 100);
    });
  });

  group('SessionStatusLine', () {
    test('stays on one line when the model name is too wide', () async {
      await testNocterm('status line overflow', (tester) async {
        await tester.pumpComponent(
          SessionStatusLine(
            modelName: 'X' * 90,
            contextTokens: 100,
            contextWindow: 1000,
          ),
        );

        // The narrow viewport forces clipping; the status line must not wrap
        // into a second row.
        final lines = tester.terminalState.getText().split('\n');
        expect(lines.where((line) => line.trim().isNotEmpty), hasLength(1));
        expect(tester.terminalState, containsText('  X'));
      });
    });
  });
}

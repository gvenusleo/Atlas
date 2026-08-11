import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  group('contextUsagePercent', () {
    test('reports 0 for missing data', () {
      expect(contextUsagePercent(0, 1000), 0);
      expect(contextUsagePercent(100, 0), 0);
    });

    test('computes the raw percentage', () {
      expect(contextUsagePercent(500, 1000), 50);
    });

    test('clamps to 100', () {
      expect(contextUsagePercent(2000, 1000), 100);
    });
  });

  group('StatusLine', () {
    test('renders model, effort, and context usage', () async {
      await testNocterm('status line', (tester) async {
        await tester.pumpComponent(
          const StatusLine(
            modelName: 'DeepSeek V4 Flash',
            effortName: 'High',
            contextTokens: 500,
            contextWindow: 1000,
          ),
        );

        expect(tester.terminalState, containsText('DeepSeek V4 Flash'));
        expect(tester.terminalState, containsText('High'));
        expect(tester.terminalState, containsText('Context 50% used'));
      });
    });

    test('omits the effort segment when none is active', () async {
      await testNocterm('status line no effort', (tester) async {
        await tester.pumpComponent(
          const StatusLine(
            modelName: 'Plain Model',
            contextTokens: 0,
            contextWindow: 0,
          ),
        );

        expect(tester.terminalState, containsText('  Plain Model'));
        expect(tester.terminalState, containsText('Context 0% used'));
        expect('${tester.terminalState}', isNot(contains('High')));
      });
    });

    test('stays on one line when the model name is too wide', () async {
      await testNocterm('status line overflow', (tester) async {
        await tester.pumpComponent(
          StatusLine(
            modelName: 'X' * 90,
            contextTokens: 100,
            contextWindow: 1000,
          ),
        );

        // The narrow viewport forces clipping; the status line must not wrap
        // into a second row.
        final lines = '${tester.terminalState}'.split('\n');
        expect(lines.where((line) => line.trim().isNotEmpty), hasLength(1));
        expect(tester.terminalState, containsText('  X'));
      });
    });
  });
}

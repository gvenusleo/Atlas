import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  test('truncates long descriptions to one line', () async {
    await testNocterm('popup truncation', (tester) async {
      await tester.pumpComponent(
        const SlashPopup(
          matches: [
            SlashCommand(
              name: 'check',
              description:
                  'Reviews code diffs, issue queues, release readiness, '
                  'commits, pushes, publishing, and project audits.',
            ),
          ],
          selected: 0,
        ),
      );

      final text = tester.terminalState.getText();
      expect(text, contains('...'));
      expect(text, isNot(contains('project audits')));
    });
  });
}

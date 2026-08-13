import 'package:atlas_tui/atlas_tui.dart';
import 'package:nocterm/nocterm.dart';
import 'package:test/test.dart';

void main() {
  test('aligns descriptions in a column across visible rows', () async {
    await testNocterm('popup alignment', (tester) async {
      await tester.pumpComponent(
        const SlashPopup(
          matches: [
            SlashCommand(name: 'model', description: 'Choose a model'),
            SlashCommand(name: 'new', description: 'Start a new session'),
          ],
          selected: 0,
        ),
      );

      final lines = tester.terminalState.getText().split('\n');
      final description1 = lines
          .firstWhere((line) => line.contains('Choose'))
          .indexOf('Choose');
      final description2 = lines
          .firstWhere((line) => line.contains('Start'))
          .indexOf('Start');
      // '/model' is wider than '/new'; the shorter name is padded so both
      // descriptions start at the same column.
      expect(description1, description2);
      expect(description1, greaterThan(0));
    });
  });

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

  test('keeps short descriptions unchanged', () async {
    await testNocterm('popup short description', (tester) async {
      await tester.pumpComponent(
        const SlashPopup(
          matches: [
            SlashCommand(name: 'new', description: 'Start a new session'),
          ],
          selected: 0,
        ),
      );

      final text = tester.terminalState.getText();
      expect(text, contains('Start a new session'));
      expect(text, isNot(contains('...')));
    });
  });
}

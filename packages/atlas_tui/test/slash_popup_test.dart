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
}

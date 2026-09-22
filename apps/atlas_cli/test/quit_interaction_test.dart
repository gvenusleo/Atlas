import 'package:test/test.dart';

import '../integration_test/support/quit_interaction.dart';

void main() {
  const prompt = '\x1b[2J\x1b[30;1H› Message Atlas (Enter to send)\x1b[30;3H';

  test('waits for the command caret before sending Enter separately', () {
    final quit = QuitInteraction();
    expect(quit.advance('Message Atlas'), isNull);
    expect(quit.advance(prompt), '/quit ');
    expect(quit.advance(prompt), isNull);
    const partial = '$prompt\x1b[30;3H/\x1b[30;4Hq';
    expect(quit.advance(partial), isNull);
    const rendered =
        '$partial\x1b[30;5Hu\x1b[30;6Hi\x1b[30;7Ht\x1b[30;8H \x1b[30;9H';
    expect(quit.advance(rendered), '\r');
    expect(quit.advance(rendered), isNull);
    expect(quit.stage, 'waiting-for-exit');
  });

  test('popup text and clipboard copies cannot acknowledge the command', () {
    final quit = QuitInteraction();
    expect(quit.advance(prompt), '/quit ');
    expect(
      quit.advance(
        '$prompt\x1b]52;c;L3F1aXQg\x07\x1b[28;1H/quit Quit Atlas\x1b[30;3H',
      ),
      isNull,
    );
    expect(quit.stage, 'waiting-for-command-caret:6');
  });

  test('fragmented typing waits for each character to appear', () {
    final quit = QuitInteraction(fragmented: true);
    var output = prompt;
    const command = '/quit ';
    for (var i = 0; i < command.length; i++) {
      expect(quit.advance(output), command[i]);
      expect(quit.advance(output), isNull);
      output += '\x1b[30;${i + 3}H${command[i]}\x1b[30;${i + 4}H';
    }
    expect(quit.advance(output), '\r');
  });

  test('incomplete cursor escape does not acknowledge unrendered input', () {
    final quit = QuitInteraction();
    expect(quit.advance(prompt), '/quit ');
    expect(quit.advance('$prompt\x1b[30;'), isNull);
  });
}

import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

void main() {
  group('slashTokenAt', () {
    test('locates the token in the middle of a draft', () {
      final token = slashTokenAt('fix bugs /he now', 11);
      expect(token, isNotNull);
      expect(token!.start, 9);
      expect(token.end, 12);
      expect(token.query, 'he');
    });

    test('ignores invalid command names', () {
      expect(slashTokenAt('/héllo', 6), isNull);
      expect(slashTokenAt('/he lp', 5), isNull);
    });

    test('handles multiline text', () {
      final token = slashTokenAt('line one\n/he', 10);
      expect(token, isNotNull);
      expect(token!.start, 9);
      expect(token.end, 12);
    });

    test('flags a token with surrounding content as skills-only', () {
      expect(slashTokenAt('fix bugs /he now', 11)!.skillsOnly, isTrue);
      expect(slashTokenAt('/he now', 3)!.skillsOnly, isTrue);
      expect(slashTokenAt('line one\n/he', 10)!.skillsOnly, isTrue);
    });
  });

  group('SlashCompleter', () {
    test('ranks exact, prefix, then substring', () {
      final completer = SlashCompleter();
      completer.sync('/n', 2);
      expect(completer.matches.map((c) => c.name), ['new']);
      completer.sync('/e', 2);
      expect(completer.matches.map((c) => c.name), ['model', 'new', 'resume']);
    });

    test('closes when the cursor leaves the token', () {
      final completer = SlashCompleter();
      completer.sync('/mo', 3);
      expect(completer.active, isTrue);
      completer.sync('/mo ', 4);
      expect(completer.active, isFalse);
      completer.sync('', 0);
      expect(completer.active, isFalse);
    });

    test('limits a skills-only token to skill commands', () {
      final completer = SlashCompleter(
        commands: [
          ...slashCommands,
          const SlashCommand(
            name: 'check',
            description: 'Review code.',
            isSkill: true,
          ),
        ],
      );
      // A leading token completes the whole catalog including built-ins.
      completer.sync('/comp', 5);
      expect(completer.matches.map((command) => command.name), ['compact']);
      // A token with other content in the draft completes skills only.
      completer.sync('/c now', 2);
      expect(completer.matches.map((command) => command.name), ['check']);
      completer.sync('/check help', 6);
      expect(completer.matches.map((command) => command.name), ['check']);
    });

    test('dismiss stays closed for the same draft and reopens on change', () {
      final completer = SlashCompleter();
      completer.sync('/', 1);
      completer.dismiss('/');
      expect(completer.active, isFalse);
      completer.sync('/', 1);
      expect(completer.active, isFalse);
      completer.sync('/m', 2);
      expect(completer.active, isTrue);
    });

    test('applyToken replaces only the token and keeps the draft', () {
      final completer = SlashCompleter();
      completer.sync('fix /mo now', 7);
      final result = completer.applyToken('fix /mo now', 'model');
      expect(result.text, 'fix /model now');
      expect(result.offset, 11);
    });
  });
}

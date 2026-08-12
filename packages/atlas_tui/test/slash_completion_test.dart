import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

void main() {
  group('slashTokenAt', () {
    test('locates a slash token at the cursor', () {
      final token = slashTokenAt('/he', 3);
      expect(token, isNotNull);
      expect(token!.start, 0);
      expect(token.end, 3);
      expect(token.query, 'he');
    });

    test('locates the token in the middle of a draft', () {
      final token = slashTokenAt('fix bugs /he now', 11);
      expect(token, isNotNull);
      expect(token!.start, 9);
      expect(token.end, 12);
      expect(token.query, 'he');
    });

    test('ignores tokens without a leading slash', () {
      expect(slashTokenAt('hello', 5), isNull);
      expect(slashTokenAt('a/b', 3), isNull);
    });

    test('ignores a cursor right after whitespace', () {
      expect(slashTokenAt('/help ', 6), isNull);
      expect(slashTokenAt('/help x', 6), isNull);
    });

    test('ignores invalid command names', () {
      expect(slashTokenAt('/héllo', 6), isNull);
      expect(slashTokenAt('/he lp', 5), isNull);
    });

    test('accepts an empty query', () {
      final token = slashTokenAt('/', 1);
      expect(token, isNotNull);
      expect(token!.query, '');
    });

    test('handles multiline text', () {
      final token = slashTokenAt('line one\n/he', 10);
      expect(token, isNotNull);
      expect(token!.start, 9);
      expect(token.end, 12);
    });
  });

  group('SlashCompleter', () {
    test('shows all commands for an empty query', () {
      final completer = SlashCompleter();
      completer.sync('/', 1);
      expect(completer.active, isTrue);
      expect(completer.matches.map((c) => c.name), [
        'compact',
        'model',
        'new',
        'quit',
        'resume',
      ]);
      expect(completer.selected, 0);
    });

    test('ranks exact, prefix, then substring', () {
      final completer = SlashCompleter();
      completer.sync('/n', 2);
      expect(completer.matches.map((c) => c.name), ['new']);
      completer.sync('/e', 2);
      expect(completer.matches.map((c) => c.name), ['model', 'new', 'resume']);
    });

    test('showAll activates with the whole catalog', () {
      final completer = SlashCompleter();
      completer.showAll();
      expect(completer.active, isTrue);
      expect(completer.matches.map((c) => c.name), [
        'compact',
        'model',
        'new',
        'quit',
        'resume',
      ]);
      expect(completer.selected, 0);
    });

    test('resets the selection when the query changes', () {
      final completer = SlashCompleter();
      completer.sync('/', 1);
      completer.move(2);
      expect(completer.selected, 2);
      completer.sync('/h', 2);
      expect(completer.selected, 0);
    });

    test('keeps the selection within bounds', () {
      final completer = SlashCompleter();
      completer.sync('/', 1);
      completer.move(-5);
      expect(completer.selected, 0);
      completer.move(10);
      expect(completer.selected, 4);
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

    test('applyToken appends a trailing space at the end', () {
      final completer = SlashCompleter();
      completer.sync('/h', 2);
      final result = completer.applyToken('/h', 'model');
      expect(result.text, '/model ');
      expect(result.offset, 7);
    });

    test('applyToken moves past an existing space', () {
      final completer = SlashCompleter();
      completer.sync('/h ', 2);
      final result = completer.applyToken('/h ', 'model');
      expect(result.text, '/model ');
      expect(result.offset, 7);
    });
  });
}

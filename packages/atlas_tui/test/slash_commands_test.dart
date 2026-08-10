import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

void main() {
  group('parseSlashCommand', () {
    test('matches built-in commands exactly', () {
      expect(parseSlashCommand('/model')?.name, 'model');
      expect(parseSlashCommand('/new')?.name, 'new');
      expect(parseSlashCommand('/quit')?.name, 'quit');
    });

    test('allows surrounding whitespace', () {
      expect(parseSlashCommand('  /new  ')?.name, 'new');
    });

    test('rejects normal messages, unknown, and malformed input', () {
      expect(parseSlashCommand('hello'), isNull);
      expect(parseSlashCommand('/unknown'), isNull);
      expect(parseSlashCommand('/'), isNull);
      expect(parseSlashCommand('/new please'), isNull);
      expect(parseSlashCommand(''), isNull);
    });
  });

  group('validSlashCommandName', () {
    test('accepts letters, digits, and separators', () {
      expect(validSlashCommandName('help'), isTrue);
      expect(validSlashCommandName('new_session'), isTrue);
      expect(validSlashCommandName('model-2'), isTrue);
      expect(validSlashCommandName('a.b'), isTrue);
    });

    test('rejects empty and invalid characters', () {
      expect(validSlashCommandName(''), isFalse);
      expect(validSlashCommandName('he lp'), isFalse);
      expect(validSlashCommandName('he/lp'), isFalse);
      expect(validSlashCommandName('héllo'), isFalse);
    });
  });
}

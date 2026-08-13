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

  group('compactCommandInstruction', () {
    test('matches a bare compact command', () {
      expect(compactCommandInstruction('/compact'), '');
      expect(compactCommandInstruction('  /compact  '), '');
    });

    test('matches a compact command with an instruction', () {
      expect(compactCommandInstruction('/compact keep files'), 'keep files');
      expect(compactCommandInstruction('/compact\tfocus'), 'focus');
    });

    test('rejects normal messages and lookalike commands', () {
      expect(compactCommandInstruction('hello'), isNull);
      expect(compactCommandInstruction('/compactness'), isNull);
      expect(compactCommandInstruction('/model'), isNull);
    });
  });

  group('resumeCommandSessionID', () {
    test('matches a bare resume command', () {
      expect(resumeCommandSessionID('/resume'), '');
      expect(resumeCommandSessionID('  /resume  '), '');
    });

    test('matches a resume command with a session id', () {
      expect(resumeCommandSessionID('/resume abc123'), 'abc123');
      expect(resumeCommandSessionID('/resume\txyz'), 'xyz');
    });

    test('rejects normal messages and lookalike commands', () {
      expect(resumeCommandSessionID('hello'), isNull);
      expect(resumeCommandSessionID('/resumable'), isNull);
      expect(resumeCommandSessionID('/quit'), isNull);
    });
  });

  group('selectedSkillNames', () {
    test('collects every slash token in order and deduplicates', () {
      expect(selectedSkillNames('/check then /write'), ['check', 'write']);
      expect(selectedSkillNames('/check /check again'), ['check']);
    });

    test('excludes built-in command names', () {
      expect(selectedSkillNames('/compact /model /check'), ['check']);
    });

    test('rejects invalid tokens and returns empty for plain text', () {
      expect(selectedSkillNames('/héllo'), isEmpty);
      expect(selectedSkillNames('hello world'), isEmpty);
      expect(selectedSkillNames(''), isEmpty);
    });
  });
}

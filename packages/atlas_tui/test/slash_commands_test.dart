import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

void main() {
  group('parseSlashCommand', () {
    test('rejects normal messages, unknown, and malformed input', () {
      expect(parseSlashCommand('hello'), isNull);
      expect(parseSlashCommand('/unknown'), isNull);
      expect(parseSlashCommand('/'), isNull);
      expect(parseSlashCommand('/new please'), isNull);
      expect(parseSlashCommand(''), isNull);
    });
  });

  group('validSlashCommandName', () {
    test('rejects empty and invalid characters', () {
      expect(validSlashCommandName(''), isFalse);
      expect(validSlashCommandName('he lp'), isFalse);
      expect(validSlashCommandName('he/lp'), isFalse);
      expect(validSlashCommandName('héllo'), isFalse);
    });
  });

  group('compactCommandInstruction', () {
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
    test('rejects normal messages and lookalike commands', () {
      expect(resumeCommandSessionID('hello'), isNull);
      expect(resumeCommandSessionID('/resumable'), isNull);
      expect(resumeCommandSessionID('/quit'), isNull);
    });
  });

  group('selectedSkillNames', () {
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

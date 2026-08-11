import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:atlas_tui/atlas_tui.dart';
import 'package:test/test.dart';

final _catalog = _MemorySkillCatalog(const [
  Skill(
    name: 'check',
    description: 'Review code.',
    dir: '/skills/check',
    path: '/skills/check/SKILL.md',
    content: '# Check instructions',
  ),
]);

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

  group('parseSkillCommand', () {
    test('matches a leading skill command', () {
      expect(parseSkillCommand('/check', _catalog)?.name, 'check');
      expect(parseSkillCommand('/check review this', _catalog)?.name, 'check');
    });

    test('allows surrounding whitespace', () {
      expect(parseSkillCommand('  /check  ', _catalog)?.name, 'check');
    });

    test('rejects unknown skills and normal messages', () {
      expect(parseSkillCommand('/unknown', _catalog), isNull);
      expect(parseSkillCommand('hello', _catalog), isNull);
      expect(parseSkillCommand('', _catalog), isNull);
      expect(parseSkillCommand('/', _catalog), isNull);
    });

    test('returns null without a catalog', () {
      expect(parseSkillCommand('/check', null), isNull);
    });
  });
}

final class _MemorySkillCatalog implements SkillCatalog {
  _MemorySkillCatalog(this.skills);

  final List<Skill> skills;

  @override
  List<SkillSummary> get summaries => [
    for (final skill in skills)
      SkillSummary(
        name: skill.name,
        path: skill.path,
        description: skill.description,
      ),
  ];

  @override
  Skill? lookup(String name) {
    for (final skill in skills) {
      if (skill.name == name) {
        return skill;
      }
    }
    return null;
  }
}

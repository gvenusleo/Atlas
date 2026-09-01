import 'dart:io';

import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:atlas_runtime/atlas_runtime.dart';
import 'package:test/test.dart';

void main() {
  late Directory root;
  late Directory userAtlas;
  late Directory userAgents;
  late Directory projectAgents;
  late Directory projectAtlas;

  setUp(() {
    root = Directory.systemTemp.createTempSync('skill_loader_test');
    userAtlas = Directory('${root.path}/.atlas/skills')
      ..createSync(recursive: true);
    userAgents = Directory('${root.path}/.agents/skills')
      ..createSync(recursive: true);
    projectAtlas = Directory('${root.path}/proj/.atlas/skills')
      ..createSync(recursive: true);
    projectAgents = Directory('${root.path}/proj/.agents/skills')
      ..createSync(recursive: true);
  });

  tearDown(() => root.deleteSync(recursive: true));

  void writeSkill(String dir, String name, String body) {
    File('$dir/$name/SKILL.md')
      ..createSync(recursive: true)
      ..writeAsStringSync(body);
  }

  FileSkillCatalog load() => loadSkillCatalog(
    homeDirectory: root.path,
    workingDirectory: '${root.path}/proj',
  );

  test('loads skills from all four roots in priority order', () {
    writeSkill(
      userAtlas.path,
      'alpha',
      '---\nname: alpha\ndescription: From user atlas.\n---\nContent A',
    );
    writeSkill(
      userAgents.path,
      'beta',
      '---\nname: beta\ndescription: From user agents.\n---\nContent B',
    );
    writeSkill(
      projectAgents.path,
      'gamma',
      '---\nname: gamma\ndescription: From project agents.\n---\nContent C',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['alpha', 'beta', 'gamma']);
    expect(catalog.lookup('gamma')!.content, contains('Content C'));
  });

  test('later roots override earlier roots for duplicate names', () {
    writeSkill(
      userAtlas.path,
      'dup',
      '---\nname: dup\ndescription: User atlas version.\n---\nUser content',
    );
    writeSkill(
      projectAgents.path,
      'dup',
      '---\nname: dup\ndescription: Project version.\n---\nProject content',
    );

    final catalog = load();
    expect(catalog.lookup('dup')!.content, contains('Project content'));
    expect(catalog.summaries.single.description, 'Project version.');
  });

  test('atlas roots override agents roots at the same level', () {
    writeSkill(
      userAgents.path,
      'dup',
      '---\nname: dup\ndescription: User agents.\n---\nUser agents content',
    );
    writeSkill(
      userAtlas.path,
      'dup',
      '---\nname: dup\ndescription: User atlas.\n---\nUser atlas content',
    );
    writeSkill(
      projectAgents.path,
      'proj',
      '---\nname: proj\ndescription: Project agents.\n---\nProject agents content',
    );
    writeSkill(
      projectAtlas.path,
      'proj',
      '---\nname: proj\ndescription: Project atlas.\n---\nProject atlas content',
    );

    final catalog = load();
    expect(catalog.lookup('dup')!.content, contains('User atlas content'));
    expect(catalog.lookup('proj')!.content, contains('Project atlas content'));
  });

  test('unquotes quoted frontmatter values', () {
    writeSkill(
      userAgents.path,
      'quoted',
      '---\nname: "quoted"\ndescription: \'Single quoted desc\'\n---\nBody',
    );

    final catalog = load();
    expect(catalog.lookup('quoted')!.description, 'Single quoted desc');
  });

  test('parses YAML block scalars with dedented multi-line values', () {
    writeSkill(
      userAgents.path,
      'block',
      '---\n'
          'name: block\n'
          'description: |\n'
          '  First line of the description.\n'
          '  Second line stays on its own line.\n'
          'allowed-tools: Bash(tvly *)\n'
          '---\n'
          'Body',
    );

    final catalog = load();
    expect(
      catalog.lookup('block')!.description,
      'First line of the description.\nSecond line stays on its own line.',
    );
  });

  test('block scalar values may contain colons', () {
    writeSkill(
      userAgents.path,
      'colons',
      '---\n'
          'name: colons\n'
          'description: |\n'
          '  Fetch the page at https://example.com and extract it.\n'
          '---\n'
          'Body',
    );

    final catalog = load();
    expect(
      catalog.lookup('colons')!.description,
      'Fetch the page at https://example.com and extract it.',
    );
  });

  test('folds lines in a `>` block scalar like YAML', () {
    writeSkill(
      userAgents.path,
      'folded',
      '---\n'
          'name: folded\n'
          'description: >\n'
          '  Folded into\n'
          '  one line.\n'
          '---\n'
          'Body',
    );

    final catalog = load();
    expect(catalog.lookup('folded')!.description, 'Folded into one line.');
  });

  test('block scalars accept tab indentation', () {
    writeSkill(
      userAgents.path,
      'tabbed',
      '---\n'
          'name: tabbed\n'
          'description: |\n'
          '\tTab-indented description.\n'
          '---\n'
          'Body',
    );

    final catalog = load();
    expect(catalog.lookup('tabbed')!.description, 'Tab-indented description.');
  });

  test('block scalars keep blank lines inside the block', () {
    writeSkill(
      userAgents.path,
      'blank',
      '---\n'
          'name: blank\n'
          'description: |\n'
          '  First paragraph.\n'
          '\n'
          '  Second paragraph.\n'
          '---\n'
          'Body',
    );

    final catalog = load();
    expect(
      catalog.lookup('blank')!.description,
      'First paragraph.\n\nSecond paragraph.',
    );
  });

  test('skips a block scalar with no content', () {
    writeSkill(
      userAgents.path,
      'emptyblock',
      '---\nname: emptyblock\ndescription: |\n---\nBody',
    );
    writeSkill(
      userAgents.path,
      'good',
      '---\nname: good\ndescription: Fine.\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['good']);
    expect(catalog.lookup('emptyblock'), isNull);
  });

  test('skips directories without SKILL.md and missing roots', () {
    Directory('${userAtlas.path}/empty').createSync();
    writeSkill(
      userAgents.path,
      'only',
      '---\nname: only\ndescription: Only one.\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['only']);
  });

  test('hides skills with disable-model-invocation', () {
    writeSkill(
      userAgents.path,
      'hidden',
      '---\nname: hidden\ndescription: Hidden skill.\ndisable-model-invocation: true\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries, isEmpty);
    expect(catalog.lookup('hidden'), isNull);
  });

  test('skips SKILL.md without frontmatter', () {
    writeSkill(userAgents.path, 'bad', 'just plain text without frontmatter');
    writeSkill(
      userAgents.path,
      'good',
      '---\nname: good\ndescription: Fine.\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['good']);
    expect(catalog.lookup('bad'), isNull);
  });

  test('skips SKILL.md missing name or description', () {
    writeSkill(
      userAgents.path,
      'noname',
      '---\ndescription: No name here.\n---\nBody',
    );
    writeSkill(
      userAgents.path,
      'good',
      '---\nname: good\ndescription: Fine.\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['good']);
  });

  test('skips oversized SKILL.md', () {
    final padding = 'x' * (Skill.maxBytes + 1);
    writeSkill(
      userAgents.path,
      'huge',
      '---\nname: huge\ndescription: Big.\n---\n$padding',
    );
    writeSkill(
      userAgents.path,
      'good',
      '---\nname: good\ndescription: Fine.\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['good']);
  });

  test('skips SKILL.md with an oversized description', () {
    final long = 'd' * (maxSkillDescriptionBytes + 1);
    writeSkill(
      userAgents.path,
      'longdesc',
      '---\nname: longdesc\ndescription: $long\n---\nBody',
    );
    writeSkill(
      userAgents.path,
      'good',
      '---\nname: good\ndescription: Fine.\n---\nBody',
    );

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['good']);
  });

  test('parses frontmatter containing YAML sequence items', () {
    // Sequence entries such as `allowed-tools` lists are not consumed by the
    // catalog; their lines must not fail the whole skill.
    writeSkill(
      userAgents.path,
      'listed',
      '---\n'
          'name: listed\n'
          'description: |\n'
          '  Multi-line description.\n'
          'allowed-tools:\n'
          '  - Bash(ls *)\n'
          '  - Bash(git status)\n'
          '---\nBody',
    );

    final catalog = load();
    expect(catalog.lookup('listed')!.description, 'Multi-line description.');
    expect(catalog.summaries.map((s) => s.name), ['listed']);
  });

  test('loads skills from symlinked directories', () {
    writeSkill(
      Directory('${root.path}/real').path,
      'linked',
      '---\nname: linked\ndescription: From a symlink.\n---\nBody',
    );
    Link('${userAgents.path}/linked').createSync('${root.path}/real/linked');

    final catalog = load();
    expect(catalog.summaries.map((s) => s.name), ['linked']);
    expect(catalog.lookup('linked')!.description, 'From a symlink.');
  });
}

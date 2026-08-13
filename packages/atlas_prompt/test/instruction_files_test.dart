import 'dart:io';

import 'package:atlas_prompt/atlas_prompt.dart';
import 'package:test/test.dart';

void main() {
  late Directory home;
  late Directory cwd;

  setUp(() async {
    final base = await Directory.systemTemp.createTemp('prompt_test_');
    home = Directory('${base.path}/home');
    cwd = Directory('${base.path}/project');
    await home.create(recursive: true);
    await cwd.create(recursive: true);
    addTearDown(() => base.delete(recursive: true));
  });

  test('loads the global and current-directory instruction files', () {
    File('${home.path}/.atlas/AGENTS.md').createSync(recursive: true);
    File('${home.path}/.atlas/AGENTS.md').writeAsStringSync('global rules');
    File('${cwd.path}/AGENTS.md').writeAsStringSync('project rules');

    final files = loadInstructionFiles(
      workingDirectory: cwd.path,
      homeDirectory: home.path,
    );

    expect(files, hasLength(2));
    expect(files.first.content, 'global rules');
    expect(files.last.content, 'project rules');
  });

  test('skips missing instruction files', () {
    final files = loadInstructionFiles(
      workingDirectory: cwd.path,
      homeDirectory: home.path,
    );

    expect(files, isEmpty);
  });

  test('deduplicates identical global and current paths', () {
    // Overlap the working directory with the global instruction path so the
    // same real file is reachable through both probe paths.
    final cwdInHome = Directory('${home.path}/.atlas');
    cwdInHome.createSync(recursive: true);
    File('${cwdInHome.path}/AGENTS.md').writeAsStringSync('rules');

    final files = loadInstructionFiles(
      workingDirectory: cwdInHome.path,
      homeDirectory: home.path,
    );

    expect(files, hasLength(1));
    expect(files.single.content, 'rules');
  });

  test('truncates oversized instruction content', () {
    File('${cwd.path}/AGENTS.md').writeAsStringSync('x' * (64 * 1024 + 100));

    final files = loadInstructionFiles(
      workingDirectory: cwd.path,
      homeDirectory: home.path,
    );

    expect(files.single.content.length, 64 * 1024);
  });
}

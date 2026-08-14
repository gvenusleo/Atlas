import 'dart:io';

import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = WriteTool();

  test('creates a file with parent directories', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'path': 'nested/deep/file.txt',
      'content': 'hello',
    });

    expect(result.isError, isFalse);
    expect(
      File('${dir.path}/nested/deep/file.txt').readAsStringSync(),
      'hello',
    );
  });

  test('overwrites an existing file completely', () async {
    final dir = await tempDir();
    await File('${dir.path}/a.txt').writeAsString('old content');

    final result = await tool.execute(toolContext(dir), {
      'path': 'a.txt',
      'content': 'new content',
    });

    expect(result.isError, isFalse);
    expect(File('${dir.path}/a.txt').readAsStringSync(), 'new content');
  });

  test('resolves absolute paths', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'path': '${dir.path}/abs.txt',
      'content': 'x',
    });

    expect(result.isError, isFalse);
    expect(File('${dir.path}/abs.txt').existsSync(), isTrue);
  });

  test('rejects an empty path', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {'content': 'x'});

    expect(result.isError, isTrue);
    expect(result.content, contains('path is required'));
  });

  test('reports diff metadata when overwriting an existing file', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/a.txt');
    await file.writeAsString('old content');

    final result = await tool.execute(toolContext(dir), {
      'path': 'a.txt',
      'content': 'new content',
    });

    expect(result.isError, isFalse);
    expect(result.metadata['path'], '${dir.path}/a.txt');
    expect(result.metadata['oldText'], 'old content');
    expect(result.metadata['newText'], 'new content');
  });

  test('reports no old text for newly created files', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'path': 'fresh.txt',
      'content': 'hello',
    });

    expect(result.isError, isFalse);
    expect(result.metadata['path'], '${dir.path}/fresh.txt');
    expect(result.metadata.containsKey('oldText'), isFalse);
    expect(result.metadata['newText'], 'hello');
  });

  test('omits diff metadata for oversized content', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'path': 'huge.txt',
      'content': 'x' * (toolDiffContentLimit + 1),
    });

    expect(result.isError, isFalse);
    expect(result.metadata, isEmpty);
  });
}

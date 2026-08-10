import 'dart:convert';
import 'dart:io';

import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = ReadTool();

  test('reads a UTF-8 file from the working directory', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/note.txt');
    await file.writeAsString('line 1\nline 2\nline 3\n');

    final result = await tool.execute(toolContext(dir), {'path': 'note.txt'});

    expect(result.isError, isFalse);
    expect(result.content, 'line 1\nline 2\nline 3');
    expect(result.metadata, isEmpty);
  });

  test('continues with offset and limit and reports next_offset', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/big.txt');
    await file.writeAsString(
      List.generate(10, (index) => 'line ${index + 1}').join('\n'),
    );

    final result = await tool.execute(toolContext(dir), {
      'path': 'big.txt',
      'offset': 3,
      'limit': 4,
    });

    expect(result.content, 'line 3\nline 4\nline 5\nline 6');
    expect(result.metadata['next_offset'], 7);
  });

  test('does not report next_offset at the end of the file', () async {
    final dir = await tempDir();
    await File('${dir.path}/small.txt').writeAsString('a\nb\n');

    final result = await tool.execute(toolContext(dir), {
      'path': 'small.txt',
      'offset': 2,
      'limit': 10,
    });

    expect(result.content, 'b');
    expect(result.metadata, isEmpty);
  });

  test('rejects a directory', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {'path': '.'});

    expect(result.isError, isTrue);
    expect(result.content, contains('directory'));
  });

  test('rejects a missing file', () async {
    final dir = await tempDir();

    final result = await tool.execute(toolContext(dir), {
      'path': 'missing.txt',
    });

    expect(result.isError, isTrue);
    expect(result.content, contains('not found'));
  });

  test('rejects non-UTF-8 content', () async {
    final dir = await tempDir();
    await File('${dir.path}/binary.bin').writeAsBytes([0xFF, 0xFE, 0x00, 0x01]);

    final result = await tool.execute(toolContext(dir), {'path': 'binary.bin'});

    expect(result.isError, isTrue);
    expect(result.content, contains('UTF-8'));
  });

  test('truncates output to the byte limit with a next_offset', () async {
    final dir = await tempDir();
    final lines = List.generate(
      5000,
      (index) =>
          'line ${index.toString().padLeft(4, '0')} '
          '${'x' * 40}',
    );
    await File('${dir.path}/huge.txt').writeAsString('${lines.join('\n')}\n');

    final result = await tool.execute(toolContext(dir), {'path': 'huge.txt'});

    expect(result.isError, isFalse);
    expect(utf8.encode(result.content).length, lessThanOrEqualTo(50 * 1024));
    expect(result.metadata['next_offset'], isNotNull);
  });

  test('rejects an invalid offset or limit', () async {
    final dir = await tempDir();
    await File('${dir.path}/a.txt').writeAsString('a\n');

    final badOffset = await tool.execute(toolContext(dir), {
      'path': 'a.txt',
      'offset': 0,
    });
    final badLimit = await tool.execute(toolContext(dir), {
      'path': 'a.txt',
      'limit': 2001,
    });

    expect(badOffset.isError, isTrue);
    expect(badLimit.isError, isTrue);
  });
}

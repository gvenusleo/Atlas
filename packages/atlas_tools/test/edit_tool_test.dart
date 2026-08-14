import 'dart:io';

import 'package:atlas_tools/atlas_tools.dart';
import 'package:test/test.dart';

import 'tool_test_utils.dart';

void main() {
  final tool = EditTool();

  test('applies multiple replacements in one call', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/code.dart');
    await file.writeAsString('final a = 1;\nfinal b = 2;\n');

    final result = await tool.execute(toolContext(dir), {
      'path': 'code.dart',
      'edits': [
        {'old_text': 'final a = 1;', 'new_text': 'final a = 10;'},
        {'old_text': 'final b = 2;', 'new_text': 'final b = 20;'},
      ],
    });

    expect(result.isError, isFalse);
    expect(file.readAsStringSync(), 'final a = 10;\nfinal b = 20;\n');
  });

  test('rejects an old_text that occurs more than once', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/dup.txt');
    await file.writeAsString('same\nsame\n');

    final result = await tool.execute(toolContext(dir), {
      'path': 'dup.txt',
      'edits': [
        {'old_text': 'same', 'new_text': 'other'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, contains('more than once'));
    expect(file.readAsStringSync(), 'same\nsame\n');
  });

  test('rejects a missing old_text and leaves the file unchanged', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/none.txt');
    await file.writeAsString('original');

    final result = await tool.execute(toolContext(dir), {
      'path': 'none.txt',
      'edits': [
        {'old_text': 'absent', 'new_text': 'x'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, contains('not found'));
    expect(file.readAsStringSync(), 'original');
  });

  test('rejects overlapping edits', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/overlap.txt');
    await file.writeAsString('abcdef');

    final result = await tool.execute(toolContext(dir), {
      'path': 'overlap.txt',
      'edits': [
        {'old_text': 'abcd', 'new_text': 'X'},
        {'old_text': 'bcd', 'new_text': 'Y'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, contains('overlap'));
    expect(file.readAsStringSync(), 'abcdef');
  });

  test('preserves a UTF-8 BOM', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/bom.txt');
    await file.writeAsBytes([
      0xEF, 0xBB, 0xBF, // BOM
      ...'hello world'.codeUnits,
    ]);

    final result = await tool.execute(toolContext(dir), {
      'path': 'bom.txt',
      'edits': [
        {'old_text': 'hello', 'new_text': 'goodbye'},
      ],
    });

    expect(result.isError, isFalse);
    final bytes = file.readAsBytesSync();
    expect(bytes.sublist(0, 3), [0xEF, 0xBB, 0xBF]);
    expect(String.fromCharCodes(bytes.sublist(3)), 'goodbye world');
  });

  test('preserves CRLF line endings', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/crlf.txt');
    await file.writeAsString('first\r\nsecond\r\nthird\r\n');

    final result = await tool.execute(toolContext(dir), {
      'path': 'crlf.txt',
      'edits': [
        {'old_text': 'second', 'new_text': 'updated'},
      ],
    });

    expect(result.isError, isFalse);
    expect(file.readAsStringSync(), 'first\r\nupdated\r\nthird\r\n');
  });

  test('rejects a non-UTF-8 file', () async {
    final dir = await tempDir();
    await File('${dir.path}/bin').writeAsBytes([0xFF, 0xFE, 0x00]);

    final result = await tool.execute(toolContext(dir), {
      'path': 'bin',
      'edits': [
        {'old_text': 'x', 'new_text': 'y'},
      ],
    });

    expect(result.isError, isTrue);
    expect(result.content, contains('UTF-8'));
  });

  test('reports diff metadata with the first replacement line', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/code.dart');
    await file.writeAsString('line one\nline two\nline three\n');

    final result = await tool.execute(toolContext(dir), {
      'path': 'code.dart',
      'edits': [
        {'old_text': 'line two', 'new_text': 'line TWO'},
      ],
    });

    expect(result.isError, isFalse);
    expect(result.metadata['path'], '${dir.path}/code.dart');
    expect(result.metadata['oldText'], 'line one\nline two\nline three\n');
    expect(result.metadata['newText'], 'line one\nline TWO\nline three\n');
    expect(result.metadata['line'], 2);
  });

  test('omits diff metadata for oversized files', () async {
    final dir = await tempDir();
    final file = File('${dir.path}/huge.txt');
    await file.writeAsString('x' * (toolDiffContentLimit + 1) + '\ntarget\n');

    final result = await tool.execute(toolContext(dir), {
      'path': 'huge.txt',
      'edits': [
        {'old_text': 'target', 'new_text': 'replaced'},
      ],
    });

    expect(result.isError, isFalse);
    expect(result.metadata, isEmpty);
  });
}

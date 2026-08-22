import 'dart:io';

import 'package:atlas_flutter/features/workspace/data/file_browser_service.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory root;
  const service = FileBrowserService();

  setUp(() {
    root = Directory.systemTemp.createTempSync('atlas_file_service_test');
  });

  tearDown(() {
    if (root.existsSync()) {
      root.deleteSync(recursive: true);
    }
  });

  test('createFile rejects names that escape the workspace', () async {
    await expectLater(
      service.createFile(
        directory: root,
        name: '../outside.txt',
        root: root.path,
      ),
      throwsA(isA<FileSystemException>()),
    );
  });

  test('rename rejects a colliding name', () async {
    final file = File('${root.path}/a.txt')..writeAsStringSync('a');
    File('${root.path}/b.txt').writeAsStringSync('b');
    await expectLater(
      service.rename(entity: file, name: 'b.txt', root: root.path),
      throwsA(isA<FileSystemException>()),
    );
  });

  test('copyInto assigns a free name on collision', () async {
    final file = File('${root.path}/a.txt')..writeAsStringSync('payload');
    final copied = await service.copyInto(
      source: file,
      directory: root,
      root: root.path,
    );
    expect(copied.path.endsWith('a 2.txt'), isTrue);
    expect(File(copied.path).readAsStringSync(), 'payload');
  });

  test('moveInto refuses to move a folder into itself', () async {
    final folder = Directory('${root.path}/sub')..createSync();
    await expectLater(
      service.moveInto(source: folder, directory: folder, root: root.path),
      throwsA(isA<FileSystemException>()),
    );
  });

  test('trashCommand uses RecycleOption for Windows directories', () {
    final command = trashCommand(
      r'C:\workspace\folder',
      isDirectory: true,
      os: 'windows',
    );
    expect(command.executable, 'powershell');
    expect(command.arguments.last, contains('DeleteDirectory'));
    expect(command.arguments.last, contains('RecycleOption'));
    expect(command.arguments.last, contains('SendToRecycleBin'));
  });

  test('trashCommand uses DeleteFile for Windows files', () {
    final command = trashCommand(
      r'C:\workspace\a.txt',
      isDirectory: false,
      os: 'windows',
    );
    expect(command.arguments.last, contains('DeleteFile'));
    expect(command.arguments.last, isNot(contains('DeleteDirectory')));
  });

  test('canonicalFilePath follows a symlink out of the workspace', () {
    final outside = Directory.systemTemp.createTempSync('atlas_outside');
    addTearDown(() {
      if (outside.existsSync()) {
        outside.deleteSync(recursive: true);
      }
    });
    final link = Link('${root.path}/escape')..createSync(outside.path);
    expect(canonicalFilePath(link.path), outside.resolveSymbolicLinksSync());
  }, skip: Platform.isWindows);

  test(
    'trashPath rejects a symlink that points outside the workspace',
    () async {
      final outside = Directory.systemTemp.createTempSync('atlas_outside');
      addTearDown(() {
        if (outside.existsSync()) {
          outside.deleteSync(recursive: true);
        }
      });
      Link('${root.path}/escape').createSync(outside.path);
      await expectLater(
        service.trashPath('${root.path}/escape', root.path),
        throwsA(isA<FileSystemException>()),
      );
    },
    skip: Platform.isWindows,
  );
}

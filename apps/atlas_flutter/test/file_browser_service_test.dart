import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

import 'package:atlas_flutter/features/workspace/data/file_browser_service.dart';

void main() {
  late Directory root;
  const service = FileBrowserService();

  setUp(() async {
    root = await Directory.systemTemp.createTemp('atlas-file-browser-');
  });

  tearDown(() async {
    if (await root.exists()) {
      await root.delete(recursive: true);
    }
  });

  test('lists directories before files in case-insensitive order', () async {
    await Directory('${root.path}/z-folder').create();
    await Directory('${root.path}/A-folder').create();
    await File('${root.path}/b.txt').writeAsString('b');
    await File('${root.path}/a.txt').writeAsString('a');

    final entries = await service.listDirectory(root);

    expect(
      entries.map(
        (entry) => entry.path
            .split(Platform.pathSeparator)
            .lastWhere((segment) => segment.isNotEmpty),
      ),
      ['A-folder', 'z-folder', 'a.txt', 'b.txt'],
    );
  });

  test('rejects binary previews', () async {
    final file = await File('${root.path}/binary.dat').writeAsBytes([0, 1]);

    expect(() => service.readPreview(file), throwsA(isA<FormatException>()));
  });

  test('rejects previews above the size limit', () async {
    final file = await File(
      '${root.path}/large.txt',
    ).writeAsString('x' * (FileBrowserLimits.previewBytes + 1));

    expect(() => service.readPreview(file), throwsA(isA<FormatException>()));
  });
}

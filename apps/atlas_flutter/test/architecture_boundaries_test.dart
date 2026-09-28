import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'features do not import runtime composition or concrete agent adapters',
    () {
      final files = _sources('lib/features');
      expect(files, isNotEmpty);
      final forbidden = RegExp(
        r'''(?:import|export)\s+['"][^'"]*(?:app/bootstrap/|package:atlas_(?:composition|config|storage|provider|tools|acp)/)''',
      );
      for (final file in files) {
        expect(
          forbidden.hasMatch(file.readAsStringSync()),
          isFalse,
          reason: file.path,
        );
      }
    },
  );

  test('shared code does not depend on features or app bootstrap', () {
    final files = _sources('lib/shared');
    expect(files, isNotEmpty);
    final forbidden = RegExp(
      r'''(?:import|export)\s+['"][^'"]*(?:features/|app/)''',
    );
    for (final file in files) {
      expect(
        forbidden.hasMatch(file.readAsStringSync()),
        isFalse,
        reason: file.path,
      );
    }
  });

  test('presentation depends on its application and models, not plugins', () {
    final files = _sources('lib/features')
        .where((file) => file.path.contains('/presentation/'));
    final forbidden = RegExp(
      r'''(?:import|export)\s+['"][^'"]*(?:/data/|package:shared_preferences/|package:flutter_secure_storage/|package:file_picker/|package:pty2/)''',
    );
    for (final file in files) {
      expect(
        forbidden.hasMatch(file.readAsStringSync()),
        isFalse,
        reason: file.path,
      );
    }
  });

  test('data and domain never depend on presentation', () {
    final files = _sources('lib/features').where(
      (file) => file.path.contains('/data/') || file.path.contains('/domain/'),
    );
    for (final file in files) {
      expect(
        RegExp(r'''(?:import|export)\s+['"][^'"]*/presentation/''')
            .hasMatch(file.readAsStringSync()),
        isFalse,
        reason: file.path,
      );
    }
  });

  test('domain models do not depend on application or data adapters', () {
    final files = _sources('lib/features')
        .where((file) => file.path.contains('/domain/'));
    final forbidden = RegExp(
      r'''(?:import|export)\s+['"][^'"]*/(?:application|data|presentation)/''',
    );
    for (final file in files) {
      expect(
        forbidden.hasMatch(file.readAsStringSync()),
        isFalse,
        reason: file.path,
      );
    }
  });

  test('browser and connection views delegate persistence and watches', () {
    const paths = [
      'lib/features/files/presentation/file_browser.dart',
      'lib/features/connections/presentation/connections_settings.dart',
      'lib/features/workspace/presentation/widgets/remote_working_directory_bar.dart',
      'lib/features/connections/presentation/remote_connect_view.dart',
    ];
    final forbidden = RegExp(
      r'RemoteConnectionStore\(|AcpConnectionStore\(|loadAcpConnections\(|saveAcpConnections\(|\.watch\(\)\.listen|widget\.service\.|FileSystemEntity\.type',
    );
    for (final path in paths) {
      expect(
        forbidden.hasMatch(File(path).readAsStringSync()),
        isFalse,
        reason: path,
      );
    }
  });
}

List<File> _sources(String path) =>
    Directory(path)
        .listSync(recursive: true)
        .whereType<File>()
        .where((file) => file.path.endsWith('.dart'))
        .toList();

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  test(
    'features do not import runtime composition or concrete agent adapters',
    () {
      final files = _sources('lib/features');
      expect(files, isNotEmpty);
      final forbidden = RegExp(
        r'''(?:import|export)\s+['"][^'"]*(?:app/(?:runtime_environment|acp_bootstrap|remote_bootstrap)|package:atlas_(?:composition|config|storage|provider|tools|acp)/)''',
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

  test('browser and connection views delegate persistence and watches', () {
    const paths = [
      'lib/features/workspace/presentation/widgets/file_browser.dart',
      'lib/features/workspace/presentation/widgets/connections_settings.dart',
      'lib/features/workspace/presentation/widgets/remote_working_directory_bar.dart',
      'lib/features/remote_connection/presentation/remote_connect_view.dart',
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

List<File> _sources(String path) => Directory(path)
    .listSync(recursive: true)
    .whereType<File>()
    .where((file) => file.path.endsWith('.dart'))
    .toList();

import 'dart:convert';
import 'dart:io';

import 'package:atlas_flutter/features/connections/data/acp_connection_codec.dart';
import 'package:atlas_flutter/features/connections/data/connection_repository.dart';
import 'package:atlas_flutter/features/connections/domain/acp_connection.dart';

/// Asynchronous file adapter for saved ACP subprocess connections.
final class const AcpConnectionStore({
  /// Home directory override; null uses the process environment.
  final String? home,
}) implements ConnectionStore<AcpConnection> {
  /// Creates a store, optionally rooted at [home] for tests.
  this;

  @override
  Future<List<AcpConnection>> load() async {
    final file = _connectionsFile(home);
    if (!await file.exists()) return const [];
    try {
      final decoded = jsonDecode(await file.readAsString());
      if (decoded is! List) return const [];
      return [for (final entry in decoded) ?AcpConnectionCodec.decode(entry)];
    } on FormatException {
      return const [];
    }
  }

  @override
  Future<void> save(List<AcpConnection> connections) async {
    final file = _connectionsFile(home);
    await file.parent.create(recursive: true);
    await file.writeAsString(
      jsonEncode([
        for (final connection in connections)
          AcpConnectionCodec.encode(connection),
      ]),
    );
  }
}

File _connectionsFile(String? home) {
  final directory =
      home ?? Platform.environment['HOME'] ?? Directory.current.path;
  return File('$directory/.atlas/acp_connections.json');
}

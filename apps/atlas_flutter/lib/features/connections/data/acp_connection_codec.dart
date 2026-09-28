import 'package:atlas_flutter/features/connections/domain/acp_connection.dart';

/// Converts persisted ACP records to connection models.
abstract final class AcpConnectionCodec {
  /// Encodes a connection for storage.
  static Map<String, Object?> encode(AcpConnection connection) => {
    'name': connection.name,
    'command': connection.command,
    'arguments': connection.arguments,
  };

  /// Parses a connection from [json], or returns null when malformed.
  static AcpConnection? decode(Object? json) {
    if (json case {'name': String name, 'command': String command}
        when name.isNotEmpty && command.isNotEmpty) {
      final rawArguments = json['arguments'];
      final arguments = rawArguments is List
          ? rawArguments.whereType<String>().toList()
          : const <String>[];
      return AcpConnection(name: name, command: command, arguments: arguments);
    }
    return null;
  }
}

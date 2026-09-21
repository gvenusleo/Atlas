import 'dart:convert';
import 'dart:io';

import 'connection_repository.dart';

/// A configured ACP server connection for the remote client mode.
final class AcpConnection {
  /// Creates an ACP connection.
  const AcpConnection({
    required this.name,
    required this.command,
    this.arguments = const <String>[],
  });

  /// Display name shown in the client.
  final String name;

  /// The executable that serves ACP over stdio (for example `atlas acp`).
  final String command;

  /// Extra arguments passed to [command].
  final List<String> arguments;

  /// Serializes this connection for storage.
  Map<String, Object?> toJson() => {
    'name': name,
    'command': command,
    'arguments': arguments,
  };

  /// Parses a connection from [json], or returns null when malformed.
  static AcpConnection? fromJson(Object? json) {
    if (json is! Map) {
      return null;
    }
    final name = json['name'];
    final command = json['command'];
    if (name is! String ||
        name.isEmpty ||
        command is! String ||
        command.isEmpty) {
      return null;
    }
    final rawArguments = json['arguments'];
    final arguments = rawArguments is List
        ? rawArguments.whereType<String>().toList()
        : const <String>[];
    return AcpConnection(name: name, command: command, arguments: arguments);
  }
}

/// Asynchronous file adapter for saved ACP subprocess connections.
final class AcpConnectionStore implements ConnectionStore<AcpConnection> {
  /// Creates a store, optionally rooted at [home] for tests.
  const AcpConnectionStore({this.home});

  /// Home directory override; null uses the process environment.
  final String? home;

  @override
  Future<List<AcpConnection>> load() async {
    final file = _connectionsFile(home);
    if (!await file.exists()) return const [];
    try {
      final decoded = jsonDecode(await file.readAsString());
      if (decoded is! List) return const [];
      return [for (final entry in decoded) ?AcpConnection.fromJson(entry)];
    } on FormatException {
      return const [];
    }
  }

  @override
  Future<void> save(List<AcpConnection> connections) async {
    final file = _connectionsFile(home);
    await file.parent.create(recursive: true);
    await file.writeAsString(
      jsonEncode([for (final connection in connections) connection.toJson()]),
    );
  }
}

File _connectionsFile(String? home) {
  final directory =
      home ?? Platform.environment['HOME'] ?? Directory.current.path;
  return File('$directory/.atlas/acp_connections.json');
}

/// Preset connection for the local Atlas ACP server.
const atlasPreset = AcpConnection(
  name: 'Atlas',
  command: 'atlas',
  arguments: ['acp'],
);

/// Preset connection for the Gemini CLI ACP server.
const geminiPreset = AcpConnection(
  name: 'Gemini CLI',
  command: 'npx',
  arguments: ['-y', '@google/gemini-cli', '--acp'],
);

/// Preset connection for the Claude Code ACP server.
const claudePreset = AcpConnection(
  name: 'Claude Code',
  command: 'npx',
  arguments: ['-y', '@agentclientprotocol/claude-agent-acp'],
);

/// Preset connection for the Codex ACP server.
const codexPreset = AcpConnection(
  name: 'Codex',
  command: 'npx',
  arguments: ['-y', '@zed-industries/codex-acp'],
);

/// The built-in connection presets, in display order.
const acpPresets = [atlasPreset, geminiPreset, claudePreset, codexPreset];

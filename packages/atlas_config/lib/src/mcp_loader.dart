part of 'config_loader.dart';

List<McpServerConfig> _mcpServers(
  Map<String, Object?> document,
  Map<String, String> env,
  String? home,
) {
  _keys(document, {'mcpServers'}, 'mcp');
  final servers = _map(document['mcpServers'] ?? {}, 'mcp.mcpServers');
  return List.unmodifiable([
    for (final entry in servers.entries)
      _mcpServer(
        entry.key,
        _map(entry.value, 'mcp.mcpServers.${entry.key}'),
        env,
        home,
      ),
  ]);
}

McpServerConfig _mcpServer(
  String name,
  Map<String, Object?> map,
  Map<String, String> env,
  String? home,
) {
  final path = 'mcp.mcpServers.$name';
  if (!RegExp(r'^[A-Za-z0-9_-]{1,128}$').hasMatch(name)) {
    throw ConfigLoadException('$path is not a valid server ID');
  }
  final type =
      _optionalString(map['type'], '$path.type') ??
      (map.containsKey('command') ? 'stdio' : 'http');
  if (!{'stdio', 'http'}.contains(type)) {
    throw ConfigLoadException('$path.type must be stdio or http');
  }
  _keys(map, {
    'type',
    'enabled',
    'timeout',
    'startupTimeout',
    if (type == 'stdio') ...['command', 'args', 'cwd', 'env'],
    if (type == 'http') ...['url', 'headers'],
  }, path);
  final enabled = _boolean(map['enabled'], '$path.enabled', true);
  final startup = _integer(map['startupTimeout'], '$path.startupTimeout', 15);
  final timeout = _integer(map['timeout'], '$path.timeout', 60);
  if (startup > 9223372036854) {
    throw ConfigLoadException(
      '$path.startupTimeout exceeds supported duration',
    );
  }
  if (timeout > 9223372036854) {
    throw ConfigLoadException('$path.timeout exceeds supported duration');
  }
  if (type == 'stdio') {
    final command = _string(map['command'], '$path.command');
    if (command.trim().isEmpty || command.contains('\x00')) {
      throw ConfigLoadException('$path.command is invalid');
    }
    final args = <String>[];
    for (final raw in _list(map['args'] ?? [], '$path.args')) {
      if (raw is! String || raw.contains('\x00')) {
        throw ConfigLoadException('$path.args must contain valid strings');
      }
      args.add(raw);
    }
    final rawCwd = _optionalString(map['cwd'], '$path.cwd');
    final cwd = rawCwd == null ? null : _expandHome(rawCwd, home);
    if (cwd != null && (cwd.contains('\x00') || !p.isAbsolute(cwd))) {
      throw ConfigLoadException('$path.cwd must be absolute');
    }
    return McpStdioConfig(
      name: name,
      enabled: enabled,
      startupTimeout: Duration(seconds: startup),
      callTimeout: Duration(seconds: timeout),
      command: command,
      args: List.unmodifiable(args),
      workingDirectory: cwd,
      environment: _mcpStrings(map['env'], '$path.env', env, enabled, false),
    );
  }
  return McpHttpConfig(
    name: name,
    enabled: enabled,
    startupTimeout: Duration(seconds: startup),
    callTimeout: Duration(seconds: timeout),
    url: _url(map['url'], '$path.url', query: true),
    headers: _mcpStrings(map['headers'], '$path.headers', env, enabled, true),
  );
}

Map<String, String> _mcpStrings(
  Object? raw,
  String path,
  Map<String, String> env,
  bool enabled,
  bool headers,
) {
  final values = _strings(raw, path);
  final result = <String, String>{};
  final seen = <String>{};
  for (final entry in values.entries) {
    final key = entry.key;
    final value = enabled
        ? entry.value.replaceAllMapped(
            RegExp(r'\$\{([A-Za-z_][A-Za-z0-9_]*)\}'),
            (match) {
              final replacement = env[match[1]];
              if (replacement == null) {
                throw ConfigLoadException(
                  '$path.$key references undefined environment variable ${match[1]}',
                );
              }
              return replacement;
            },
          )
        : entry.value;
    final lower = key.toLowerCase();
    final invalid = headers
        ? !RegExp(r"^[!#$%&'*+.^_`|~0-9A-Za-z-]+$").hasMatch(key) ||
              lower.startsWith('mcp-') ||
              {
                'host',
                'content-type',
                'content-length',
                'accept',
                'last-event-id',
                'connection',
                'transfer-encoding',
              }.contains(lower) ||
              !seen.add(lower)
        : key.isEmpty || key.contains('=') || key.contains('\x00');
    if (invalid ||
        value.contains('\x00') ||
        (headers && (value.contains('\r') || value.contains('\n')))) {
      throw ConfigLoadException('$path.$key is invalid');
    }
    result[key] = value;
  }
  return Map.unmodifiable(result);
}

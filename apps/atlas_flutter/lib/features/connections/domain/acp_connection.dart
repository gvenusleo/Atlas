/// A configured ACP server connection for the remote client mode.
final class const AcpConnection({
  /// Display name shown in the client.
  required final String name,

  /// The executable that serves ACP over stdio (for example `atlas acp`).
  required final String command,

  /// Extra arguments passed to [command].
  final List<String> arguments = const <String>[],
}) {}

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

/// Capabilities exposed by an agent session to presentation clients.
final class AgentCapabilities {
  /// Creates a capability set.
  const AgentCapabilities({
    this.modes = false,
    this.slashCommands = false,
    this.rename = false,
    this.compact = false,
    this.permissions = false,
    this.images = false,
  });

  /// Whether session modes can be selected.
  final bool modes;

  /// Whether slash commands are advertised.
  final bool slashCommands;

  /// Whether sessions can be renamed remotely.
  final bool rename;

  /// Whether context compaction is available.
  final bool compact;

  /// Whether permission requests may be presented.
  final bool permissions;

  /// Whether image content is accepted.
  final bool images;
}

/// Optional capability provider implemented by local and remote sessions.
abstract interface class AgentCapabilityProvider {
  /// The capabilities currently supported by this session.
  AgentCapabilities get capabilities;
}

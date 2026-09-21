/// Capabilities exposed by an agent session to presentation clients.
final class const AgentCapabilities({
  /// Whether session modes can be selected.
  final bool modes = false,

  /// Whether slash commands are advertised.
  final bool slashCommands = false,

  /// Whether sessions can be renamed remotely.
  final bool rename = false,

  /// Whether context compaction is available.
  final bool compact = false,

  /// Whether permission requests may be presented.
  final bool permissions = false,

  /// Whether image content is accepted.
  final bool images = false,
}) {
  /// Creates a capability set.
  this;
}

/// Optional capability provider implemented by local and remote sessions.
abstract interface class AgentCapabilityProvider {
  /// The capabilities currently supported by this session.
  AgentCapabilities get capabilities;
}

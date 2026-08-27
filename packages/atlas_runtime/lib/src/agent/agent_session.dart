import 'runtime_service.dart';

/// Presentation-facing session contract shared by local and ACP clients.
///
/// It currently aliases the legacy runtime surface while callers migrate;
/// protocol-specific capability members will be split out in the next step.
abstract interface class AgentSession implements RuntimeService {}

/// The Agent Client Protocol adapter for Atlas.
///
/// Exposes the shared runtime to ACP clients over NDJSON stdio. The adapter
/// owns the JSON-RPC lifecycle and must not duplicate agent orchestration or
/// persistence.
library;

export 'src/acp_server.dart';
export 'src/acp_types.dart';
export 'src/stdio_transport.dart';
export 'src/update_mapper.dart';

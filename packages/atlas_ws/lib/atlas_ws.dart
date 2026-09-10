/// Versioned WebSocket wire contract and transport for remote Atlas clients.
///
/// Serves the shared runtime to remote ACP clients: each WebSocket text
/// frame carries one JSON-RPC message of the ACP protocol, mirroring the
/// stdio transport of `atlas acp`. The server owns the HTTP upgrade,
/// authentication, connection limits, and frame policies; it does not
/// compose runtime services.
library;

export 'src/remote_token.dart';
export 'src/ws_server.dart';

# atlas_ws

Atlas's remote WebSocket transport. It serves the shared runtime to remote ACP clients (such as the Atlas mobile app) over a versioned wire contract where each text frame carries exactly one ACP JSON-RPC message.

## Responsibility

- The `/acp` WebSocket upgrade, guarded by a bearer-token authorizer.
- One `AcpServer` per connection over the shared runtime, each with its own ACP `initialize` handshake.
- Connection and frame policies: connection limit, text-frame size limit, binary-frame rejection, ping-based dead-connection detection, and disconnect accounting.
- `RemoteTokenFile` persistence and constant-time validation of the access token (`~/.atlas/remote_token`, mode 0600).

## Allowed dependencies

- `atlas_acp` for the per-connection `AcpServer` and `atlas_runtime` public types.
- `shelf`, `shelf_web_socket`, `web_socket_channel`, `stream_channel`, and `crypto` for the upgrade, channel plumbing, and token digests.

## Prohibited ownership

- No runtime composition, session persistence, provider logic, or tool execution; the server receives the composed runtime from its caller.
- No ACP protocol implementation, CLI parsing, or configuration loading; `atlas_cli` owns the `atlas server` entry point.

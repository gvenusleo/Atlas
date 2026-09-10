# atlas_ws

Atlas's remote WebSocket transport. Serves the shared runtime to remote ACP
clients (such as the Atlas mobile app) over a versioned wire contract: each
WebSocket text frame carries exactly one ACP JSON-RPC message.

## Responsibility

- HTTP `/acp` endpoint with WebSocket upgrade, guarded by a bearer-token
  authorizer.
- Per-connection `AcpServer` lifecycle over the shared runtime: one agent
  connection per client, each with its own ACP `initialize` handshake.
- Connection and frame policies: connection limit, text-frame size limit,
  binary-frame rejection, ping-based dead-connection detection, and
  disconnect accounting.
- `RemoteTokenFile` persistence and constant-time validation of the remote
  access token (`~/.atlas/remote_token`, mode 0600).

## Allowed dependencies

- `atlas_acp` for the per-connection `AcpServer`.
- `atlas_runtime` public types (`AgentEngine`, `ModelDescriptor`).
- `shelf`, `shelf_web_socket`, `web_socket_channel`, `stream_channel` for the
  HTTP upgrade and channel plumbing, and `crypto` for token digests.

## Prohibited ownership

- No runtime composition, session persistence, provider logic, or tool
  execution; the server receives the composed runtime from its caller.
- No ACP protocol implementation; the JSON-RPC lifecycle belongs to
  `atlas_acp`.
- No CLI parsing or configuration-file loading; `atlas_cli` owns the
  `atlas server` entry point.

# ACP Protocol

[中文](zh-CN/protocol-acp.md)

Atlas implements ACP v1 in `packages/atlas_acp`. The Flutter application always uses an ACP client, including for an Atlas runtime hosted in-process.

## Supported Surface

- Session creation, prompting, cancellation, loading, resume, listing, close, deletion, and configuration options, with text, image, embedded text resource, and resource-link prompt blocks.
- Message, reasoning, tool, plan, command, session-info, and usage updates.
- Shell progress uses standard `tool_call_update` text content with `in_progress` status. Each snapshot replaces the previous content until the final result carries `completed` or `failed`, and `rawOutput` holds captured text and final metadata even for failures and replay. No terminal extension is required.
- ACP clients keep tool content when a partial update omits it, including status-only completion and history replay; explicit empty content clears it.
- Atlas sends no permission requests: tools run with the permissions of the Atlas process, and clients must not wait for an approval round trip.

## Atlas Extensions

Atlas extensions use the `_atlas.dev` namespace, declared in `agentCapabilities._meta['atlas.dev']`:

- `_atlas.dev/session/set_title` renames a persisted Atlas session.
- `compact` advertises Atlas context compaction.
- `permissionModel: none` declares that the Atlas agent never sends `session/request_permission`.

The Atlas ACP client still handles permission requests from third-party agents, because agent and client permission behavior are separate protocol roles. The runtime-facing contract is `AgentSession`; ACP-only presentation members (titles, commands, and modes) are exposed through `PresentationAgentSession`.

## WebSocket Transport

`atlas server` exposes the same ACP surface over `atlas_ws` for the Atlas mobile client:

- `GET /acp` upgrades when the request carries `Authorization: Bearer <token>`; the token is issued by `atlas server` and stored in `~/.atlas/remote_token` with mode 0600.
- Each WebSocket text frame carries exactly one ACP JSON-RPC message in either direction; binary or oversized frames close the connection.
- Lifecycle, sessions, events, permission requests, and `_atlas.dev` extensions behave as they do over stdio, with one `AcpServer` per connection over the shared runtime.
- A dropped socket cancels no turn: the runtime finishes and persists it, and the client reconciles through `session/load` after reconnecting with exponential backoff.

## Planned

Client filesystem and terminal capabilities, and ACP v2 support.

## Host-configured MCP tools

MCP tools configured on the Atlas host use the existing `tool_call` and `tool_call_update` flow, including progress and history replay, and generic ACP tool titles are retained when there is no built-in tool kind. Session-provided `mcpServers` is explicitly rejected: host configuration is global to the composed runtime and does not implement ACP session isolation. See [MCP tools](mcp.md).

# atlas_acp

Atlas's Agent Client Protocol adapter. It serves the shared runtime to ACP clients (such as Zed) over NDJSON stdio; clients launch `atlas acp` as a subprocess and drive sessions through JSON-RPC.

## Responsibility

- Owns the ACP JSON-RPC lifecycle through `acpd` (`AgentRole` / `ClientRole`) instead of a shared Atlas RPC wrapper. stdout carries only protocol messages, and logging goes to stderr.
- Exposes an in-process `AcpClient` so Flutter can consume a local `AcpServer` over an in-memory transport without spawning a process.
- Maps `session/prompt` turns to `session/update` notifications: message and thought chunks, tool calls and updates (including shell output snapshots, `rawOutput`, and `diff` content blocks for `write`/`edit`), plans, session info, and usage.
- Advertises ACP v1 capabilities for session load, resume, list, close, delete, additional directories, image and embedded-context prompts, plus `configOptions` for model and reasoning effort.
- Provides slash commands through `available_commands_update`: `/compact` for manual compaction, and one command per available skill, where `/name` tokens inject that skill's instructions.
- Handles `session/cancel` as cooperative turn cancellation.

## Allowed dependencies

- `atlas_runtime` public types, `acpd`, `acpd_io`, and `stream_channel` for the stdio and in-memory transports.

## Prohibited ownership

- No agent orchestration or persistence; the adapter maps protocol methods to runtime calls and must not duplicate runtime behavior.
- No HTTP or WebSocket transport, and no client-side terminal or filesystem implementations.

## Not implemented

Session-supplied MCP server connections, filesystem write and terminal client methods, elicitation, and HTTP/WebSocket transports; these capabilities are not advertised during initialization. Permission requests are handled on the client side (`session/request_permission`) and surfaced through `PermissionPort`. Host-configured MCP tools come from the runtime, and their connections belong to `atlas_mcp`.

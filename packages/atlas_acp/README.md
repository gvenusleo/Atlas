# atlas_acp

Atlas's Agent Client Protocol adapter.

Serves the shared runtime to ACP clients (such as Zed) over NDJSON stdio.
`atlas acp` starts the adapter as the process entry point; clients launch it
as a subprocess and drive sessions through JSON-RPC.

## Implemented

- `initialize` with ACP v1 capability negotiation, advertising `loadSession`,
  `sessionCapabilities` (`resume`, `list`, `close`, `delete`,
  `additionalDirectories`), and `promptCapabilities` (`image`,
  `embeddedContext`)
- `session/new`, `session/load` (with timeline replay), `session/resume`,
  `session/list`, `session/close`, `session/delete`
- Session `configOptions` (model and reasoning effort selectors returned by
  new/load/resume, `category: "model"` and `category: "thought_level"`) with
  `session/set_config_option` applying them to subsequent turns
- `session/prompt` mapping runtime turns to `session/update` notifications:
  `agent_message_chunk`, `agent_thought_chunk`, `tool_call` (with `rawInput`),
  `tool_call_update` (with `rawOutput`), `plan` (from the `plan` tool),
  `session_info_update` (auto-generated titles), and `usage_update`
- Prompt content: text, image (base64 data URLs), embedded text resources,
  and baseline `resource_link` blocks
- `session/cancel` mapping to cooperative turn cancellation

## Constraints

- Atlas owns the JSON-RPC lifecycle and uses `json_rpc_2` directly instead of
  a shared Atlas RPC wrapper.
- The adapter maps protocol methods to runtime calls; it must not duplicate
  agent orchestration or persistence.
- stdout carries only protocol messages; logging goes to stderr.

## Not implemented

MCP server connections, filesystem and terminal client methods, permission
requests, elicitation, session modes (superseded by `configOptions`), and
HTTP/WebSocket transports. These capabilities are not advertised during
initialization.
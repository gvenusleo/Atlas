# atlas_mcp

The Model Context Protocol adapter for Atlas.

> **Status:** Planned. No client or server implementation exists yet. The MCP
> client is the first implementation target; server support is added only
> after the client path is stable.

## Responsibility

- Adapts external tools to the tool layer through an MCP client.
- Owns the MCP JSON-RPC lifecycle and uses `json_rpc_2` directly instead of
  depending on a shared Atlas RPC wrapper.

## Allowed dependencies

- `json_rpc_2` and `atlas_runtime` public types.

## Prohibited ownership

- No agent orchestration, provider, storage, or tool implementations; the
  adapter maps protocol methods to runtime calls.

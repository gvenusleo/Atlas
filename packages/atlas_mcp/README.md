# atlas_mcp

Atlas's Model Context Protocol adapter.

The first implementation target is an MCP client for external tools. MCP server support will be added only after the runtime client path is stable.

MCP owns its JSON-RPC lifecycle and uses `json_rpc_2` directly instead of depending on a shared Atlas RPC wrapper.

# atlas_mcp

MCP client connections exposed as Atlas tools.

## Responsibility

- Connects configured stdio and Streamable HTTP servers through `mcp_dart`.
- Discovers paginated tool catalogs, creates stable provider-compatible names, and implements the runtime `Tool` port.
- Maps text, JSON, errors, progress, cancellation and deadlines to Atlas results.
- Owns idempotent connection cleanup and releases partial startup resources.
- Freezes catalogs until restart and prevents automatic replay after session loss.

## Allowed dependencies

- `atlas_runtime` public tool, cancellation and logging ports.
- `mcp_dart` for MCP protocol and transport implementation. Its `package:http` dependency is permitted for MCP transport/authentication; other Atlas HTTP integrations continue to use Dio.
- `crypto` for deterministic tool identity digests; Dart core libraries for lifecycle and result conversion.

## Prohibited ownership

- No agent loop, provider requests, storage, Flutter/UI, YAML parsing or CLI.
- No MCP server implementation or ACP session configuration.
- No automatic tool-call retry or optional client capability without an Atlas implementation.

## Verification and limits

`dart test` runs independent JSON-RPC fixtures over real subprocess pipes and local HTTP, covering legacy and modern protocols, JSON/SSE, cancellation, catalog validation, authentication errors, output limits, cleanup and replay prevention. See [MCP support](../../docs/mcp.md) for configuration and operational limits.

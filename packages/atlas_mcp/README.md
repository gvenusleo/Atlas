# atlas_mcp

MCP client connections exposed as Atlas tools.

## Responsibility

- Connects configured stdio and Streamable HTTP servers through `mcp_dart`, discovers paginated tool catalogs, creates stable provider-compatible names, and implements the runtime `Tool` port.
- Maps text, JSON, errors, progress, cancellation, and deadlines to Atlas results.
- Owns idempotent connection cleanup, releases partial startup resources, freezes catalogs until restart, and prevents automatic replay after session loss.

## Allowed dependencies

- `atlas_runtime` public tool, cancellation, and logging ports.
- `mcp_dart` for the MCP protocol and transport; its `package:http` dependency is permitted here, while other Atlas HTTP integrations use Dio.
- `crypto` for deterministic tool identity digests, plus Dart core libraries.

## Prohibited ownership

- No agent loop, provider requests, storage, UI, YAML parsing, or CLI.
- No MCP server implementation or ACP session configuration.
- No automatic tool-call retry or optional client capability without an Atlas implementation.

## Verification and limits

`dart test` runs JSON-RPC fixtures over real subprocess pipes and local HTTP, covering legacy and modern protocols, JSON/SSE, cancellation, catalog validation, authentication errors, output limits, cleanup, and replay prevention. Configuration and operational limits are documented in [MCP tools](../../docs/mcp.md).

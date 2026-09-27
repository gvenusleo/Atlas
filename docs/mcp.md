# MCP tools

[中文](zh-CN/mcp.md)

Atlas is an MCP client: tools from configured servers participate in the same agent loop as `read`, `write`, `edit`, `shell` and `plan`. It uses `mcp_dart` 2.4.2 for stdio and Streamable HTTP. MCP server support is not implemented.

## Configuration

Add `mcp_servers` to the existing `~/.atlas/config.yaml`. It is an optional list, defaulting to empty. Restart Atlas after editing it.

```yaml
mcp_servers:
  - name: local_tools
    transport: stdio
    command: /absolute/path/to/mcp-server
    args: []
    cwd: ~/workspace
    env:
      SERVICE_TOKEN: ${SERVICE_TOKEN}
    startup_timeout_seconds: 15
    call_timeout_seconds: 60

  - name: remote_tools
    transport: streamable_http
    url: https://tools.example.com/mcp
    headers:
      Authorization: Bearer ${MCP_TOKEN}
    enabled: true
```

Replace the example executable and endpoint with an installed MCP server. Atlas does not install servers or interpret `command` as a shell command. Use `args` for literal arguments. No OAuth browser login is performed; remote servers must accept no authentication or your configured static headers/token.

| Field | Rules |
|---|---|
| `name` | Unique; 1–128 ASCII letters, digits, underscores or hyphens. |
| `transport` | `stdio` or `streamable_http`. |
| `enabled` | Boolean, default `true`. Disabled entries are structurally validated but do not resolve credentials or connect. |
| `startup_timeout_seconds` | Positive integer, default 15; connection and all discovery pages share this deadline. |
| `call_timeout_seconds` | Positive integer, default 60; progress does not extend the total deadline. |
| `command`, `args` | stdio executable and optional string list; no shell expansion. |
| `cwd` | stdio only; absolute path, with leading `~/` expansion. Omitted means Atlas's startup directory. |
| `env` | stdio string overrides on the bootstrap environment snapshot. |
| `url` | HTTP(S) endpoint without userinfo or fragment; query parameters are preserved. |
| `headers` | HTTP string map; cannot override MCP protocol, Host, Content-Type, Content-Length, Accept, Last-Event-ID, Connection or Transfer-Encoding headers. |

`${VAR}` substitution is supported in `env` and `headers` values. An undefined variable in an enabled entry fails configuration loading with its field path. Header names must be valid and unique ignoring case; header values must not contain newlines. Unknown or transport-incompatible fields are rejected. Duration values must fit within Dart's supported range.

CLI inherits its startup environment. Flutter desktop uses its existing bootstrap snapshot, including macOS login-shell exports. Server `cwd` is fixed for the connection lifetime; it does not follow changes to an Atlas session's directory.

## Ownership and startup

The CLI TUI, `atlas acp`, `atlas server`, and Flutter desktop local mode each compose their own MCP connections. Mobile and remote clients use the tools configured on the host they connect to; they do not start MCP processes or forward local MCP credentials. Each host runtime shares one connection per server across its sessions.

Servers connect in configuration order before the runtime becomes available. Failure of any enabled server fails startup with its name and a redacted failure category, closing connections already opened. Disable the failing entry and restart to work without it. Help, version, cache reporting and token rotation do not open MCP connections.

Process shutdown drains active turns before closing MCP connections, provider HTTP clients and storage. SIGINT/SIGTERM during CLI startup cancels discovery and cleans up its subprocesses. A normal remote client disconnect retains running host turns. SDK subprocess cleanup targets the owned process; deliberately detached or spawned descendants are not guaranteed to be terminated.

## Tool behavior

Tool names use a readable `mcp_` prefix, sanitized server/tool names and a stable SHA-256 suffix. Names stay within 64 provider-compatible characters; original names are retained for dispatch. Servers follow configuration order and tools within each server sort by original name. The input schema and description reach the provider through the existing runtime tool descriptor.

The catalog is fixed at startup. List-change notifications produce a single `mcp.catalog_changed_restart_required` log event per connection when logging is enabled. Modern protocol peers use an acknowledged tool-list subscription. Restart to refresh tool definitions. Tools requiring Tasks execution are omitted with a safe diagnostic. Server instructions are not appended to the system prompt.

Catalog metadata is bounded, because it reaches every provider request: a server-provided description is truncated at 2 KiB UTF-8 with an explicit marker, a tool whose input schema exceeds 64 KiB fails startup, and a server whose catalog metadata exceeds 1 MiB in total fails startup. Both failures name the server and keep its definitions out of provider requests.

Results preserve ordered text blocks, embedded text resources and resource links; links are not fetched automatically. Structured JSON is also included in the model-visible output, including scalar or array results from modern peers. Images, audio and binary resources are replaced with explicit omission markers. A result containing only unsupported binary content becomes an adapter error. Empty success results remain valid.

Output and identity metadata have a combined **50 KiB UTF-8** budget. Truncation is marked in both text and metadata, without splitting UTF-8 characters. Progress uses transient bounded snapshots and is neither persisted nor fed to the model. Each call still produces one final persisted result. Existing generic tool views and ACP history replay display these results without a new UI or storage schema.

Cancellation and deadlines apply to the individual request. On stdio the connection stays usable. On Streamable HTTP the SDK cannot abort a request that is already in flight, so Atlas marks that connection unusable instead of leaking one more request per attempt: the failing call reports the timeout or cancellation, later calls to that server fail with `connection_abandoned`, and the connection is closed in the background so its remaining streams are released. The abandoned request ends when the server responds or the process exits; restart Atlas to reconnect that server. Timeouts, authentication failures, malformed results and transport failures are returned to the model as safe errors. A cancelled or disconnected operation may already have produced a remote side effect; Atlas reports uncertainty and does not automatically retry it.

## Compatibility and current limits

- The SDK negotiates its stable profile, including MCP 2026-07-28 and legacy initialization. Tests cover 2025-11-25 and 2026-07-28 over stdio and HTTP, including JSON and SSE HTTP responses. They are interoperability checks, not a claim that every optional MCP capability is supported.
- Atlas disables subprocess restart and HTTP event-stream retries, and blocks SDK session reinitialization/replay after a stale-session response. Restart Atlas after a lost connection/session. Legacy stdio peers must respond to the discovery probe or allow its timeout/fallback; a peer that exits on the probe fails startup.
- A Streamable HTTP deadline leaves exactly one request in flight on that connection; Atlas issues no further calls on it, so the residual cost is bounded by the number of configured servers rather than by the number of attempts.
- Final result limits do not bound the SDK's entire HTTP response buffering. Stdio retains the SDK's 10 MiB incoming-message limit; HTTP JSON/SSE decoding can allocate the full response before Atlas truncates the result. There is currently no Atlas transport-wide HTTP byte limit.
- Child stderr is drained but omitted from user output. Raw SDK logs are silenced; Atlas emits only safe categories through its existing logger, without configured secrets, raw protocol bodies or SDK exception text.
- ACP session-provided `mcpServers` remains unsupported and is explicitly rejected. Configure servers on the Atlas host instead. Per-session roots and configuration require separate session isolation work.
- OAuth login/token refresh, dynamic catalogs, resources/prompts as first-class features, roots, sampling, elicitation, Tasks, Apps and model-visible binary results remain Planned. The client does not advertise those optional capabilities.

See the [development plan](plans/mcp-support.md) for implementation decisions and follow-up scope, and [configuration](configuration.md) for the full Atlas schema.

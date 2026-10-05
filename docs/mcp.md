# MCP tools

[中文](zh-CN/mcp.md)

Atlas is an MCP client: tools from configured servers join the same agent loop as `read`, `write`, `edit`, `shell`, and `plan`. It uses `mcp_dart` 2.4.2 for stdio and Streamable HTTP. Atlas is not an MCP server.

## Configuration

Configure `~/.atlas/mcp.json` using the common `mcpServers` object, then restart Atlas. A missing file means no external servers. Atlas accepts the stdio/HTTP subset of Pi-style MCP configuration; unsupported OAuth and exposure options are rejected.

```json
{
  "mcpServers": {
    "local_tools": {
      "command": "/absolute/path/to/mcp-server",
      "args": [],
      "cwd": "~/workspace",
      "env": { "SERVICE_TOKEN": "${SERVICE_TOKEN}" },
      "startupTimeout": 15,
      "timeout": 60
    },
    "remote_tools": {
      "type": "http",
      "url": "https://tools.example.com/mcp",
      "headers": { "Authorization": "Bearer ${MCP_TOKEN}" },
      "enabled": true
    }
  }
}
```

Atlas does not install servers and does not treat `command` as a shell command; put literal arguments in `args`. No OAuth browser login is performed, so a remote server must accept no authentication or your configured static headers.

| Field | Rules |
|---|---|
| Object key | Server ID: 1-128 ASCII letters, digits, underscores or hyphens. |
| `type` | `stdio` or `http`; inferred from `command` when omitted, otherwise HTTP. |
| `enabled` | Boolean, default `true`. Disabled entries are structurally validated but do not resolve credentials or connect. |
| `startupTimeout` | Atlas extension: positive integer seconds, default 15; connection and discovery share this deadline. |
| `timeout` | Positive integer seconds, default 60; unlike Pi, progress does not extend Atlas's total call deadline. |
| `command`, `args` | stdio executable and optional string list; no shell expansion. |
| `cwd` | stdio only; absolute path, with leading `~/` expansion. Omitted means Atlas's startup directory. |
| `env` | stdio string overrides on the bootstrap environment snapshot. |
| `url` | HTTP(S) endpoint without userinfo or fragment; query parameters are preserved. |
| `headers` | HTTP string map; cannot override MCP protocol, Host, Content-Type, Content-Length, Accept, Last-Event-ID, Connection or Transfer-Encoding headers. |

`${VAR}` substitution works in `env` and `headers`; an undefined variable in an enabled entry fails configuration loading with its field path. Header names must be valid and unique ignoring case, header values must not contain newlines, and unknown or transport-incompatible fields are rejected.

## Ownership and startup

The CLI TUI, `atlas acp`, `atlas server`, and Flutter desktop local mode each compose their own MCP connections, and each host runtime shares one connection per server across its sessions. Mobile and remote clients use the tools configured on the host they connect to: they start no MCP processes and forward no local credentials. The CLI inherits its startup environment, and Flutter desktop uses its existing bootstrap snapshot, including macOS login-shell exports. Server `cwd` is fixed for the connection lifetime.

Servers connect in configuration order before the runtime becomes available. Any enabled server that fails aborts startup with its name and a redacted failure category, closing connections already opened; disable that entry and restart to work without it. Help, version, cache reporting, and token rotation open no connections. Process shutdown drains active turns before closing MCP connections, provider clients, and storage, and SIGINT/SIGTERM during startup cancels discovery and cleans up its subprocesses. SDK cleanup targets the processes Atlas owns; deliberately detached descendants are not guaranteed to be terminated.

## Tool behavior

Tool names combine a readable `mcp_` prefix, sanitized server and tool names, and a stable SHA-256 suffix, within 64 provider-compatible characters, while dispatch keeps the original name. Servers follow configuration order and tools sort by original name inside each server.

The catalog is fixed at startup: list-change notifications only log `mcp.catalog_changed_restart_required` once per connection, and restarting refreshes definitions. Tools that require Tasks execution are omitted with a safe diagnostic, and server instructions are not appended to the system prompt.

Because catalog metadata reaches every provider request, a description is truncated at 2 KiB UTF-8 with an explicit marker, a tool whose input schema exceeds 64 KiB fails startup, and a server whose catalog metadata exceeds 1 MiB in total fails startup. The two failures name the server and keep its definitions out of provider requests.

Results keep ordered text blocks, embedded text resources, and resource links (which are not fetched automatically), and structured JSON also reaches the model. Images, audio, and binary resources are replaced with explicit omission markers; a result containing only unsupported binary content becomes an adapter error, while an empty success result stays valid. Output and identity metadata share a 50 KiB UTF-8 budget, truncation is marked in both, and progress uses transient bounded snapshots that are neither persisted nor sent to the model. Every call still produces one final persisted result that existing generic tool views and ACP history replay display unchanged.

Cancellation and deadlines apply to the individual request. On stdio the connection stays usable. The Streamable HTTP SDK cannot abort a request already in flight, so Atlas marks that connection unusable instead of leaking one more request per attempt: the failing call reports the timeout or cancellation, later calls fail with `connection_abandoned`, and the connection closes in the background. Restart Atlas to reconnect that server. Timeouts, authentication failures, malformed results, and transport failures reach the model as safe errors, and an operation that was cancelled or disconnected may already have had a remote side effect, so Atlas reports uncertainty instead of retrying automatically.

## Compatibility and current limits

- The SDK negotiates its stable profile, including MCP 2026-07-28 and legacy initialization, and tests cover 2025-11-25 and 2026-07-28 over stdio and HTTP with JSON and SSE responses. Those are interoperability checks, not a claim that every optional MCP capability is supported.
- Subprocess restart and HTTP stream retries are disabled, and the SDK cannot replay calls after a stale-session response; restart Atlas after a lost connection. Legacy stdio peers must answer the discovery probe or allow its timeout.
- Final result limits do not bound the SDK's HTTP response buffering, and stdio keeps the SDK's 10 MiB incoming-message limit. There is no Atlas-wide HTTP byte limit.
- Child stderr is drained but not shown, and raw SDK logs are silenced: Atlas logs only safe categories, without configured secrets, raw protocol bodies, or SDK exception text.
- ACP session-provided `mcpServers` is explicitly rejected; configure servers on the Atlas host instead, since per-session roots and configuration need separate isolation work.
- OAuth login and token refresh, dynamic catalogs, resources and prompts as first-class features, roots, sampling, elicitation, Tasks, Apps, and model-visible binary results remain Planned, and the client does not advertise those capabilities.

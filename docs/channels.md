# Channels

[中文](zh-CN/channels.md)

## ACP

`atlas acp` starts an [Agent Client Protocol](https://agentclientprotocol.com/) service via stdin/stdout for ACP clients like Zed to connect.

Add Atlas in Zed:

```json
"agent_servers": {
  "Atlas": {
    "type": "custom",
    "command": "~/.local/bin/atlas",
    "args": ["acp"]
  }
}
```

ACP supported features:

- Session creation, resumption, history replay, paginated listing, deletion
- Prompt, cancel, close
- Model switching, reasoning effort switching, chain-of-thought streaming
- Embedded text resources
- Session info and usage updates
- Client terminal for `run_shell` output
- File tool locations/diff display
- Image input
- Long-term memory background worker
- `/compact` slash command
- Skill slash commands, e.g. `/think ...`
- Plan updates: `todo_write` tool calls are mapped to `plan_update` session updates, rendered as a structured plan panel in editors

When connected via ACP, Atlas prefers client-declared capabilities:

- **Terminal capability**: `run_shell` requests the client terminal to execute and embeds the output.
- **Filesystem capability**: file tools request the client to read/write files and display locations/diffs.

When the client doesn't support a capability or the call fails, Atlas falls back to local tool execution.

`additionalDirectories` are saved and returned as session metadata, but relative paths are still resolved from `cwd`. ACP auth, permission requests, MCP connections, audio, and non-image binary resource input are not currently supported.

## WeChat

`atlas weixin login` logs in via WeChat QR code and saves the account token to `~/.atlas/weixin/accounts`. `atlas weixin serve` connects to the WeChat Bot, long-polls text and image messages, and invokes the local Atlas runtime.

When the model updates its task list via `todo_write`, items with `in_progress` status are sent to the user as progress messages.

The WeChat channel has the same file and shell permissions as the local Atlas process. The working directory for the first message uses the current directory when `atlas weixin serve` starts. Only the WeChat user who logged in via QR code can control Atlas. Group chats, audio, video, and adding other controllers are not supported.

Slash commands available in WeChat chat:

| Command | Description |
|---|---|
| `/help` | Show commands |
| `/status` | Show current working directory and session |
| `/cwd` | Show current working directory |
| `/cwd /absolute/path` | Switch working directory; next regular message starts a new conversation |
| `/cwd -` | Switch back to the previous working directory |
| `/new` | Start a new conversation in the current working directory |
| `/sessions` | List recent sessions for the current working directory |
| `/sessions all` | List recent sessions across all working directories |
| `/resume <session-id>` | Resume a session and switch to its working directory |
| `/compact` | Compact current session context |
| `/cancel` | Cancel the currently running turn |

## WebSocket

`atlas serve` starts a WebSocket server for local network clients (e.g. a mobile app) to connect. The server listens on `0.0.0.0:8765` by default, configurable via `services.ws.host` and `services.ws.port` in `~/.atlas/config.json`.

The WebSocket channel is designed for LAN-only use with no authentication. It exposes the same agent capabilities as other channels:

- Prompt with text and image input
- Streaming events: model deltas, reasoning deltas, tool calls
- Session listing, detail, deletion, compaction
- Model switching and listing
- Skill summaries
- Turn cancellation
- Long-term memory background worker

### Multi-session concurrency

A single WebSocket connection supports multiple concurrent sessions. Each session maintains its own working directory, selected model, and turn state. Different sessions can run turns in parallel; the same session rejects a second prompt while a turn is running.

All messages are routed by `session_id`:

- `prompt` with `session_id` — runs in that session's context. Without `session_id`, creates a new session; the assigned ID is returned in `turn_finished`.
- `cancel` — requires `session_id`, cancels only that session's turn without affecting others.
- `set_model` — requires `session_id`, sets the model for that session only.
- All streaming events (`turn_started`, `model_delta`, `tool_started`, etc.) carry `session_id` so the client can route them to the correct session UI.

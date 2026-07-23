# Channels

[中文](zh-CN/channels.md)

## Terminal UI

Run `atlas` without a subcommand to start the full-screen terminal UI in the current working directory:

```sh
atlas
atlas --session <id>
```

The optional `--session` flag loads an existing transcript or creates that session on the first turn. The interface streams model output with terminal-rendered Markdown, keeps tool calls and results in occurrence order, supports multiline pasted input, and restores persisted history when resuming a session. While a turn or manual compaction is active, a transient row above the composer shows elapsed time. It uses `Thinking` for streamed reasoning and `Working` for model output, tool execution, or compaction. The footer shows the active model, reasoning effort, and the most recent context usage as a percentage of `context_window`.

Controls:

- `Enter` sends the current input when no command suggestion is active.
- `Shift+Enter` inserts a line break. Use `Ctrl+J` as a fallback in terminals that cannot distinguish `Shift+Enter` from `Enter`.
- Type `/` at the start of the input to see supported commands and available skills. Use the arrow keys to choose a suggestion, then `Tab` or `Enter` to complete it.
- Enter `/model` to choose a configured model and its reasoning effort. Use the arrow keys and `Enter` to select.
- Enter `/resume` to choose a saved session. Type to search by title, ID, or working directory; use the left and right arrows to switch between the current directory and all sessions, then press `Enter` to resume. `/resume <session-id>` resumes an exact ID directly.
- Enter `/compact [instruction]` to summarize earlier context while keeping recent messages. The optional instruction tells the compactor what to preserve.
- Enter `/quit` to exit the TUI.
- `Page Up`, `Page Down`, and the mouse wheel scroll conversation history.
- Drag across conversation text to select and copy it to the clipboard.
- `Esc` interrupts the active turn or manual compaction. In the resume picker it returns to the current conversation; it otherwise has no effect while idle.
- `Ctrl+C` has no effect.

The resume picker initially lists sessions saved under the current working directory and can switch to an all-session view. Resuming a session from another directory requires confirmation; after confirmation, subsequent instructions, skills, and tools use that session's saved working directory. The current TUI model and reasoning effort remain selected because sessions do not persist those settings.

The TUI starts with `default_model` and the first configured `reasoning_efforts` option. `/model` selections apply to subsequent turns for the lifetime of the current TUI process; they do not rewrite the configuration file or persist across restarts. Manual compaction preserves the full transcript while rewriting the saved context summary used by subsequent turns; its command and result notice are not persisted as conversation messages. Entering an available skill command, such as `/think plan this change`, injects that skill for the current turn while preserving the original prompt in the transcript. Image input is not available in the TUI yet.

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
- Image input
- `/compact [instruction]` slash command; the optional instruction tells the compactor what to preserve
- Skill slash commands, e.g. `/think ...`
- Plan updates: `todo_write` tool calls are mapped to `plan_update` session updates, rendered as a structured plan panel in editors

When connected via ACP, `run_shell` requests the client terminal when available and embeds its output. ACP terminals do not accept standard input, so calls with non-empty `stdin` execute through Atlas's local shell. If terminal creation is unavailable, Atlas also falls back to its local shell.

`additionalDirectories` are saved and returned as session metadata, but relative paths are still resolved from `cwd`. ACP auth, permission requests, MCP connections, audio, and non-image binary resource input are not currently supported.

## WebSocket

`atlas serve` starts a WebSocket server for clients such as a mobile app. The server listens on `127.0.0.1:8765` by default, configurable via `services.ws.host` and `services.ws.port` in `~/.atlas/config.json`.

Loopback connections do not require authentication. Binding to a non-loopback address, such as `0.0.0.0`, requires `services.ws.token`; clients must send it as `Authorization: Bearer <token>`. WebSocket handshakes also enforce same-origin checks. The channel exposes the same agent capabilities as other channels:

- Prompt with text and image input
- Streaming events: model deltas, reasoning deltas, tool calls
- Session listing, detail, deletion, compaction
- Model switching and listing
- Reasoning effort switching
- Skill summaries
- Turn cancellation

### Multi-session concurrency

A single WebSocket connection supports multiple concurrent sessions. Each session maintains its own working directory, selected model, and turn state. Different sessions can run turns in parallel; the same session rejects a second prompt while a turn is running, including prompts from another connection.

All messages are routed by `session_id`:

- `prompt` with `session_id`: runs in that session's context. Without `session_id`, creates a new session; the assigned ID is returned in `turn_finished`.
- `cancel`: requires `session_id`, cancels only that session's turn without affecting others.
- `set_model`: requires `session_id`, sets the model for that session only.
- All streaming events (`turn_started`, `model_delta`, `tool_started`, etc.) carry `session_id` so the client can route them to the correct session UI.

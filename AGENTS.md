# Atlas Development Guide

## Project Positioning

Atlas is a local general-purpose agent written in Go. The core is a testable headless agent loop for handling everyday tasks and coding tasks on the user's machine. CLI, ACP, WeChat, and WebSocket channels all call into the same capabilities via `internal/runtime`.

Core responsibilities of a single turn:

- Append user input to the transcript.
- Call the model provider with system prompt, message history, and tool definitions.
- Execute tool calls in the order returned by the model, and write results back to the transcript.
- End when there are no tool calls, an error occurs, or `max_steps` is reached.
- Save transcript, usage, context compaction, and memory-related metadata.

## Collaboration Principles

State assumptions before proposing solutions. When ambiguity arises, list possible interpretations. Choose the smallest, verifiable slice for each change. No drive-by refactors.

## Local Access Model

Atlas tools have the same local permissions as the Atlas process itself — they can read and write files, search text, and execute shell commands. Atlas does not provide a sandbox, permission prompts, or an approval gate. This is a product boundary, not a missing implementation. Do not introduce permission abstractions into the code unless the product direction changes.

When an ACP client declares terminal capability, `run_shell` can request the client terminal to execute and embed the output. When a client declares filesystem capability, file tools can request the client filesystem and display locations/diff. When the client doesn't support a capability or the call fails, fall back to Atlas local tools.

Before calling the client text file interface, ACP file tools only pass confirmed local plain UTF-8 text files. Directories, special files, and binary content fall back to local tools.

README, CLI copy, and tests must all reflect this boundary. Tests only verify tool behavior, fallback, and error propagation — not permission prompts.

## Configuration and Prompts

Atlas reads configuration from `~/.atlas/config.json`. The top-level `default_model` selects the default model by matching a `models[].value` across all providers. The `providers` array contains provider configs, each with `name`, `format`, `base_url`, `api_key`, and `models`. The `format` field selects the adapter by API format — currently `chat_completions` and `responses` are supported, defaulting to `chat_completions` when not configured. Model entries must include at least `value`, `name`, `context_window`, `max_tokens`, and `input_formats`. The `input_formats` field currently supports `text` and `image`, and must include `text`. Model `value` must be globally unique across all providers. Supported reasoning depths are declared via `reasoning_efforts`; the first option is used when not explicitly selected, and this is not placed in the global `agent` config.

`format`, `base_url`, `api_key`, and model `value` belong only to provider connection config and do not enter `model.ChatRequest`. The `agent` package only depends on `model.Provider` and generic generation parameters.

Long-term memory config includes `memory.enabled` and `memory.model`. When `memory.enabled` is not configured, it defaults to enabled. When enabled but `memory.model` is empty, background memory tasks use the model that produced the session. When configured, it must match a model in any provider's model list.

The system prompt is constructed by `internal/prompt`. It only describes model behavior, tool usage principles, verification habits, and reply style — it does not repeat tool JSON schemas.

Atlas loads only two additional instruction files:

- `~/.atlas/AGENTS.md`
- `AGENTS.md` in the current working directory

Current user requests take precedence over instruction files. Current-directory instructions take precedence over global instructions. Atlas does not recursively search parent or child directories for `AGENTS.md`.

Atlas scans user-level and current-directory-level skills, but the system prompt only injects `name` and `description`. The full `SKILL.md` can only be loaded on demand via the `load_skill` tool.

## Package Structure

Recommended package boundaries:

```text
cmd/atlas
internal/acp
internal/agent
internal/compact
internal/config
internal/memory
internal/model
internal/prompt
internal/provider/chatcompletions
internal/provider/responses
internal/runtime
internal/session
internal/skill
internal/tool
internal/transcript
internal/version
internal/weixin
internal/ws
```

Core flow:

```text
CLI / ACP / Weixin / WS
  -> runtime.RunTurn
  -> agent.RunTurn
  -> provider.Stream
  -> tool.Registry
  -> transcript + session store
```

`internal/acp` only does ACP protocol adaptation, session/update notifications, client terminal/filesystem bridging, embedded text resource parsing, and session state management. It does not duplicate the agent loop.

`internal/weixin` only does iLink Bot login, message polling, typing, slash commands, reply sending, and lightweight user-to-session binding. It does not duplicate the agent loop.

`internal/ws` only does WebSocket connection management, message dispatch, observer event mapping, and per-connection state (cwd, session, model). It does not duplicate the agent loop.

## Core Boundaries

**Provider**: `model.Provider` is the sole interface to model backends. `internal/provider/*` implements adapters by API format, e.g. Chat Completions and Responses. The provider handles connection info, authentication, model name, request format, SSE parsing, and response format conversion. `model.ChatRequest` only expresses Atlas's generic chat protocol.

**Tool**: Tools are registered via `tool.Tool`: `Definition()` returns the model-visible definition, and `Run(ctx, arguments)` parses the JSON arguments from the model and returns a text result. Tools should be stateless where possible. The registry handles sorting, duplicate checking, and execution by name.

**Transcript**: `internal/transcript` only holds the message sequence for the current agent instance. Reads return copies. It is not responsible for persistence, compaction, or summarization.

**Session**: `internal/session` uses SQLite to store transcripts, session metadata, context compaction metadata, memory extraction boundaries, and raw output items needed for provider continuation. The default path is `~/.atlas/atlas.db`, overridable via `session.db_path`. No migration framework is currently implemented; early schema changes require users to delete and recreate the old database.

**Memory**: `internal/memory` maintains long-term memory entries, summaries, FTS retrieval, and background task queues. Long-term memory shares the SQLite database with sessions but uses separate tables. The runtime triggers extraction tasks based on incremental thresholds, explicit memory instructions, or context compaction. The worker only processes new messages since the last boundary and refreshes summaries for affected scopes. Memory types remain three: `instruction`, `fact`, and `workflow`.

## Runtime Constraints

Keep the agent loop predictable:

- A turn contains numbered steps.
- Every tool call has a paired tool result.
- Tool results are in the same order the model returned them.
- Tool errors are written back to the transcript as model-visible tool results.
- Public interfaces accept `context.Context`.
- Observer events are sent in the order they occur. CLI, ACP, and future UI render in this order.

## Tradeoff Principles

Atlas pursues small, clear, and verifiable. Current real usage paths take priority over theoretical completeness.

- Don't build compatibility matrices upfront. After encountering real provider differences, pin behavior with tests, then do local adaptation.
- Don't keep duplicate interfaces, fallbacks, or config switches for "maybe later."
- Don't abstract before two real call sites exist.
- Don't import the full architecture of upstream large agents. Only adopt what makes the current core smaller and clearer.

## Testing Standards

Key behaviors are tested with fake providers and temp directories. Priority coverage areas:

- Agent loop: plain text replies, tool calls, error writeback, ordering, `max_steps`.
- Tools: file read/write, text search, shell success and failure, Tavily tools.
- Prompt: global and current-directory instructions, skill summaries, long-term memory injection.
- Session: save, restore, list, view, delete, additional working directories, context compaction, image content fragment persistence.
- Memory: schema, retrieval, enqueue, incremental extraction, summary refresh, disable, model selection, and image placeholders.
- ACP: initialization, session lifecycle, history replay, cancel, paginated listing, usage, terminal/filesystem capability, image input, `/compact`.
- WeChat: QR login, account saving, typing, image input, working directory switching, session listing, restore, compaction, cancel.
- WebSocket: prompt events, session switching, cancel, image parts, model options, session listing/detail/delete/compact, skills.

Run before delivery:

```sh
go test ./...
```

## Code Style

- Keep code small and explicit.
- Prefer the Go standard library.
- Only modify code directly related to the current task.
- Comments should be necessary, concise, and follow Go conventions.
- Exported types, functions, and packages require proper doc comments.
- Tests, docs, and comments serve behavior understanding — don't pad with explanations.
- Don't reintroduce old details from previous Atlas implementations just because they existed.

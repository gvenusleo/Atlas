# Built-in Tools and Skills

[中文](zh-CN/tools.md)

## Built-in Tools

| Tool | Description |
|---|---|
| `run_shell` | Discover, search, inspect, edit, and verify with PowerShell on Windows or `/bin/sh` elsewhere; accepts optional standard input and accepted exit codes, and local execution retains a full temporary log when bounded output is truncated |
| `load_skill` | Load a local skill's instructions by name |
| `web_search` | Search the public web with Tavily; requires `services.tavily.api_key` |
| `web_fetch` | Extract public web page content with Tavily; requires `services.tavily.api_key` |
| `todo_write` | Manage a structured task list for multi-step work; each call replaces the entire list |

## Task Tracking

The `todo_write` tool lets the model track multi-step tasks with `pending` / `in_progress` / `completed` statuses. Each call fully replaces the previous list. The model is instructed to use it for tasks that span several tool calls, and to avoid churn by only updating after real progress.

Todo state is not persisted to the database. Instead, when context compaction occurs, the last `todo_write` call is extracted from the transcript and incomplete items are injected into the summary prompt, so task state survives compaction.

Channel-specific rendering:

- **ACP**: todo updates are sent as `plan_update` session updates, mapping each entry to a `PlanEntry`. Editors like Zed render them as a structured plan panel.
- **CLI**: todos appear as a tool call in the transcript with a `Todo: N items` title.

## Instructions and Skills

Atlas loads two additional instruction files (current user requests take precedence over instruction files; current-directory instructions take precedence over global ones; parent and child directories are not searched recursively):

- `~/.atlas/AGENTS.md`
- `AGENTS.md` in the current working directory

Atlas also scans user-level and current-directory-level skills, injecting only `name` and `description` summaries into the system prompt. When full instructions are needed, the model reads the corresponding `SKILL.md` via `load_skill`. When connected via ACP, available skills are exposed as `/<skill>` commands scoped to the current session's working directory. User input is passed as-is to the model, and the full `SKILL.md` is injected directly for that turn.

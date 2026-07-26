# Built-in Tools and Skills

[中文](zh-CN/tools.md)

## Built-in Tools

| Tool | Description |
|---|---|
| `run_shell` | Discover, search, inspect, edit, and verify with PowerShell on Windows or `/bin/sh` elsewhere; requires a short user-facing purpose and command, accepts optional standard input and accepted exit codes, and local execution retains a full temporary log when bounded output is truncated |
| `load_skill` | Load a local skill's instructions by name |
| `web_search` | Search the public web with Tavily; requires `services.tavily.api_key` |
| `web_fetch` | Extract public web page content with Tavily; requires `services.tavily.api_key` |
| `update_plan` | Manage a structured task plan for multi-step work; each call replaces the entire plan |

## Plan Tracking

The `update_plan` tool lets the model track multi-step work with `pending` / `in_progress` / `completed` statuses. Each call fully replaces the previous plan. A plan can contain up to 50 steps, with at most 500 characters per step and one `in_progress` step. The model is instructed to use it for tasks that span several tool calls, and to avoid churn by only updating after real progress.

Plan updates are preserved in transcript tool calls and structured metadata. When context compaction occurs, the latest `update_plan` plan is injected into the summary prompt if it contains unfinished steps, so both completed progress and pending work survive compaction.

Channel-specific rendering:

- **ACP**: plan updates are sent as `plan_update` session updates, mapping each step to a `PlanEntry`. Editors like Zed render them as a structured plan panel.
- **TUI**: each update appears as a structured plan snapshot with completed, in-progress, and pending step styles.

## Instructions and Skills

Atlas loads two additional instruction files (current user requests take precedence over instruction files; current-directory instructions take precedence over global ones; parent and child directories are not searched recursively):

- `~/.atlas/AGENTS.md`
- `AGENTS.md` in the current working directory

Atlas also scans user-level and current-directory-level skills, injecting only `name` and `description` summaries into the system prompt. When full instructions are needed, the model reads the corresponding `SKILL.md` via `load_skill`. When connected via ACP, available skills are exposed as `/<skill>` commands scoped to the current session's working directory. User input is passed as-is to the model, and the full `SKILL.md` is injected directly for that turn.

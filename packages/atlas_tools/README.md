# atlas_tools

Built-in Atlas tool implementations: `read`, `write`, `edit`, `shell`, and `plan`.

## Responsibility

- Implements the `atlas_runtime` `Tool` and `ToolRegistry` ports with structured JSON arguments and results, and resolves relative file paths against the session working directory.
- `shell` runs platform commands with bounded live output, optional one-shot stdin, session-relative `cwd`, configurable timeouts, and cancellation.
- `plan` replaces the complete task plan for multi-step work, tracking each step as `pending`, `in_progress`, or `completed`.
- Tool behavior, limits, and the security boundary are documented in [tool behavior](../../docs/tools.md).

## Allowed dependencies

`atlas_runtime` public types only (file and process APIs come from `dart:io`).

## Prohibited ownership

- No model, provider, storage, or orchestration logic.
- No client-specific output formatting; tools return plain `ToolResult` values.
- No sandbox or permission abstractions: tools run with the local process permissions.

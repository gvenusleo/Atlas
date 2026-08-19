# atlas_tools

Built-in Atlas tool implementations: `read`, `write`, `edit`, `shell`, and
`plan`.

## Responsibility

- Implements `atlas_runtime` `Tool` and `ToolRegistry` ports with structured
  JSON arguments and results.
- File tools resolve relative paths against the session working directory.
- `shell` executes commands with the platform default shell, bounded output,
  timeout, optional `cwd` override, and cancellation support.
- `plan` replaces the complete task plan for multi-step work, tracking each
  step as `pending`, `in_progress`, or `completed`.

## Allowed dependencies

`atlas_runtime` public types only (file and process APIs come from `dart:io`).

## Prohibited ownership

- No model, provider, storage, or orchestration logic.
- No client-specific output formatting; tools return plain `ToolResult`
  values.
- No sandbox or permission abstractions: tools run with the local process
  permissions.
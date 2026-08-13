# atlas_tools

Built-in Atlas tool implementations: `read`, `write`, `edit`, and `shell`.

## Responsibility

- Implements `atlas_runtime` `Tool` and `ToolRegistry` ports with structured
  JSON arguments and results.
- File tools resolve relative paths against the session working directory.
- `shell` executes commands with the platform default shell, bounded output,
  timeout, and cancellation support.

## Allowed dependencies

`atlas_runtime` public types only (file and process APIs come from `dart:io`).

## Prohibited ownership

- No model, provider, storage, or orchestration logic.
- No client-specific output formatting; tools return plain `ToolResult`
  values.
- No sandbox or permission abstractions: tools run with the local process
  permissions.
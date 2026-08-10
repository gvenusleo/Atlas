# atlas_cli

The command-line and Nocterm entry point for Atlas.

## Responsibility

- Owns the process composition root: `composeRuntime` loads `atlas_config`,
  constructs provider, tool, and storage adapters, and injects the
  `atlas_prompt` system prompt builder into one `atlas_runtime` `AgentRuntime`
  instance.
- Will start the Nocterm TUI by default; the planned `atlas server` subcommand
  exposes the composed runtime through `atlas_ws`. Other non-interactive
  commands share the same runtime instead of duplicating the agent loop.

## Allowed dependencies

- `atlas_config`, `atlas_prompt`, `atlas_provider`, `atlas_storage`,
  `atlas_tools`, and `atlas_runtime` for composition.
- `dart:io` for file, process, and entry-point access.

## Prohibited ownership

- No re-implementation of the agent loop; every client uses the single
  `atlas_runtime` engine.
- No rendering logic; the Nocterm UI belongs to `atlas_tui`.
- No protocol logic: `atlas_ws`, `atlas_acp`, and `atlas_mcp` adapters are not
  owned here.
- No provider-specific request fields, persistence schemas, or tool
  implementations; those belong to their owning packages.
# atlas_cli

The command-line and Nocterm entry point for Atlas.

## Responsibility

- Owns the process entry point: loads `~/.atlas/config.yaml` and calls
  `atlas_composition` `composeRuntime` to obtain one `atlas_runtime`
  `AgentRuntime` instance.
- Starts the Nocterm TUI by default (`atlas`); `atlas acp` serves the same
  runtime to ACP clients over NDJSON stdio. The planned `atlas server`
  subcommand will expose the composed runtime through `atlas_ws`. Other
  non-interactive commands share the same runtime instead of duplicating the
  agent loop.

## Allowed dependencies

- `atlas_composition` for runtime construction.
- `atlas_config` to load the configuration file.
- `atlas_prompt` for `loadSkillCatalog` at the TUI entry.
- `atlas_tui` for the default chat interface.
- `atlas_acp` for the `atlas acp` server.
- `atlas_runtime` public types.
- `dart:io` for file, process, and entry-point access.

## Prohibited ownership

- No re-implementation of the agent loop; every client uses the single
  `atlas_runtime` engine.
- No rendering logic; the Nocterm UI belongs to `atlas_tui`.
- No protocol logic: Planned WebSocket and MCP adapters are not owned here;
  `atlas_acp` is started from this process but implemented in its own package.
- No provider-specific request fields, persistence schemas, or tool
  implementations; those belong to their owning packages. Composition of
  those adapters belongs to `atlas_composition`.

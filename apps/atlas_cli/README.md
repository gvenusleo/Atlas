# atlas_cli

The command-line and Nocterm entry point for Atlas.

## Responsibility

- Owns the process entry point: loads `~/.atlas/config.yaml`, then calls `atlas_composition` `composeTools` and `composeRuntime` to obtain one `AgentRuntime`.
- Starts the Nocterm TUI by default (`atlas`); `atlas acp` serves the same runtime to ACP clients over NDJSON stdio, and `atlas server` exposes it over a bearer-token WebSocket endpoint for mobile and remote clients. Other subcommands reuse the same runtime instead of duplicating the agent loop.
- Routes commands through a thin executable trampoline and `CommandRunner<int>`, validating help and usage before configuration or resource allocation.
- Owns resource teardown: drains runtime shutdown, then closes protocol listeners, MCP connections, storage, and HTTP clients before returning a standard exit code.
- Generates `--version` from this package's pubspec with `build_version`.

## Verification

`dart test` covers command behavior in memory, and `dart test integration_test` builds and tests a native bundle (or reuses `ATLAS_TEST_BINARY`). Terminal requirements, exit codes, and version generation are documented in [Development](../../docs/development.md).

## Allowed dependencies

- `atlas_composition` for runtime construction, plus `atlas_config` and `atlas_prompt` for configuration and skill loading at the TUI entry.
- `atlas_tui` for the chat interface, `atlas_acp` for `atlas acp`, and `atlas_ws` for the `atlas server` endpoint and token store.
- `atlas_runtime` public types, with `atlas_storage` and `atlas_provider` used only for owned adapter lifetimes.
- `args`, `io`, `stack_trace`, and `dart:io` for routing, exit codes, diagnostics, and entry-point access.

## Prohibited ownership

- No re-implementation of the agent loop; every client uses the single `atlas_runtime` engine.
- No rendering logic (belongs to `atlas_tui`) and no protocol logic; WebSocket and MCP adapters are not owned here.
- No provider-specific request fields, persistence schemas, or tool implementations; composing those adapters belongs to `atlas_composition`.

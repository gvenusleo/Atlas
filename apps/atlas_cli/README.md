# atlas_cli

The command-line and Nocterm entry point for Atlas.

## Responsibility

- Owns the process entry point: loads `~/.atlas/config.yaml` and calls
  `atlas_composition` `composeRuntime` to obtain one `atlas_runtime`
  `AgentRuntime` instance.
- Starts the Nocterm TUI by default (`atlas`); `atlas acp` serves the same
  runtime to ACP clients over NDJSON stdio. `atlas server` exposes the same
  runtime over a WebSocket endpoint (`atlas_ws`) guarded by a bearer token,
  for the mobile and remote clients. Other non-interactive commands share the
  same runtime instead of duplicating the agent loop.

- Uses a thin executable trampoline and `CommandRunner<int>` routing. Help and
  usage validation precede configuration and resource allocation.
- Owns resource teardown: drains runtime shutdown, closes protocol listeners,
  storage, and HTTP clients, then returns a standard process exit code.
- Generates `--version` from this package's pubspec with `build_version`.

## Verification

`dart test` covers command behavior in memory. `dart test integration_test`
builds and tests a native bundle (or uses `ATLAS_TEST_BINARY`). macOS/Linux
PTY tests require Python 3. See [Development](../../docs/development.md) for
terminal requirements, exit codes, and version generation.

## Allowed dependencies

- `atlas_composition` for runtime construction.
- `atlas_config` to load the configuration file.
- `atlas_prompt` for `loadSkillCatalog` at the TUI entry.
- `atlas_tui` for the default chat interface.
- `atlas_acp` for the `atlas acp` server.
- `atlas_ws` for the `atlas server` WebSocket endpoint and token store.
- `atlas_runtime` public types.
- `atlas_storage` and `atlas_provider` for owned adapter lifetimes, not schemas
  or provider request logic.
- `args`, `io`, and `stack_trace` for routing, exit codes, and diagnostics.
- `dart:io` for file, process, and entry-point access.

## Prohibited ownership

- No re-implementation of the agent loop; every client uses the single
  `atlas_runtime` engine.
- No rendering logic; the Nocterm UI belongs to `atlas_tui`.
- No protocol logic: WebSocket and Planned MCP adapters are not owned here;
  `atlas_acp` is started from this process but implemented in its own package.
- No provider-specific request fields, persistence schemas, or tool
  implementations; those belong to their owning packages. Composition of
  those adapters belongs to `atlas_composition`.

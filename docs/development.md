# Development

[中文](zh-CN/development.md)

## Current State

The repository is a Dart and Flutter workspace scaffold. The existing Flutter shell is executable; runtime, daemon, CLI, TUI, ACP, and MCP behavior remain planned.

## Workspace Layout

```text
packages/atlas_core       domain models and ports
packages/atlas_runtime    shared agent engine
packages/atlas_storage    SQLite persistence
packages/atlas_provider   model provider adapters
packages/atlas_tools      built-in tools
packages/atlas_rpc        generic JSON-RPC support
packages/atlas_protocol   client wire protocol
packages/atlas_acp        ACP adapter
packages/atlas_mcp        MCP adapter
packages/atlas_tui        Nocterm client
apps/atlasd               local runtime host
apps/atlas_cli            CLI and terminal entry point
apps/atlas_flutter        Flutter desktop and mobile client
```

The root Pub workspace owns the only `pubspec.lock`. Workspace members use `resolution: workspace` and must not add member lockfiles.

## Toolchain

The root `mise.toml` pins Flutter 3.44.9, which provides Dart 3.12.2.

```sh
mise install
just deps
```

Use `just deps-update` only when intentionally changing dependency constraints or the lockfile.

## Verification

```sh
just fmt          # format Dart sources
just fmt-check    # check formatting without rewriting
just analyze      # analyze the workspace
just test         # run available Dart and Flutter tests
just ci           # complete repository verification
```

Run the current Flutter shell with `just app-run macos`. Platform debug builds use the matching `just app-build-*` recipe.

## Package Rules

- Put code in the package that owns the behavior; do not collect unrelated helpers in `atlas_core`.
- Add public abstractions only when a real adapter or test requires them.
- Public Dart APIs require concise documentation comments.
- Runtime and protocol packages must not import Flutter.
- Client packages must not import provider, tool, or storage implementations.
- Generated serialization files stay beside their source and are committed only when the selected generator requires it.
- Add focused tests with behavior. Empty scaffold packages do not need placeholder tests.

## Documentation Rules

- Root README files contain product status and supported commands, not internal architecture.
- Architecture and dependency boundaries belong in `docs/architecture.md`.
- Mark unavailable behavior as `Planned`; remove stale examples when behavior is removed.
- Keep English and Chinese counterparts synchronized.
- Record high-impact, difficult-to-reverse decisions under `docs/decisions`.

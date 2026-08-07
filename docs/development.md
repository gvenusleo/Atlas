# Development

[中文](zh-CN/development.md)

## Current State

The repository is a Dart and Flutter workspace scaffold. The existing Flutter shell is executable; runtime, CLI, WebSocket transport, TUI, ACP, and MCP behavior remain planned.

## Workspace Layout

```text
packages/atlas_runtime    domain models, ports, and shared agent engine
packages/atlas_storage    Drift persistence
packages/atlas_provider   model provider adapters
packages/atlas_tools      built-in tools
packages/atlas_protocol   client wire protocol
packages/atlas_ws         WebSocket transport
packages/atlas_acp        ACP adapter
packages/atlas_mcp        MCP adapter
packages/atlas_tui        Nocterm presentation package
apps/atlas_cli            atlas CLI, TUI, server, and other commands
apps/atlas_flutter        Flutter desktop and mobile application
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

- Put domain concepts and runtime ports in `atlas_runtime`; keep provider, storage, tool, UI, and protocol implementations in their owning packages.
- Add public abstractions only when a real adapter or test requires them.
- Public Dart APIs require concise documentation comments.
- Runtime and protocol packages must not import Flutter.
- Presentation packages must not import provider, tool, or storage implementations.
- Application bootstrap code in `atlas_cli` and `atlas_flutter` composes those adapters and injects the runtime.
- `atlas_ws` owns WebSocket transport only and accepts an injected request handler.
- Generated serialization files stay beside their source and are committed only when the selected generator requires it.
- Add focused tests with behavior. Empty scaffold packages do not need placeholder tests.

## Documentation Rules

- Root README files contain product status and supported commands, not internal architecture.
- Architecture and dependency boundaries belong in `docs/architecture.md`.
- Mark unavailable behavior as `Planned`; remove stale examples when behavior is removed.
- Keep English and Chinese counterparts synchronized.
- Record high-impact, difficult-to-reverse decisions under `docs/decisions`.

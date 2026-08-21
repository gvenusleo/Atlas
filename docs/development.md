# Development

[中文](zh-CN/development.md)

## Current State

The repository is a Dart and Flutter workspace. `atlas_runtime`, `atlas_storage`, the provider adapters, `atlas_config`, `atlas_tools`, `atlas_prompt`, `atlas_composition`, the `atlas_tui` Nocterm chat interface, the ACP server adapter (`atlas_acp`), and local Flutter runtime composition are executable with focused tests; the MCP adapter and WebSocket transport remain planned.

## Workspace Layout

```text
packages/atlas_runtime       Session/Turn domain, timeline, ports, and agent engine
packages/atlas_storage       Drift persistence and runtime row mapping
packages/atlas_provider      model provider adapters
packages/atlas_config        YAML config loading and validation
packages/atlas_prompt        system prompt and skill catalog loading
packages/atlas_composition   shared runtime composition for CLI and Flutter
packages/atlas_tools         built-in tools
packages/atlas_ws            versioned WebSocket protocol and transport (Planned)
packages/atlas_acp           ACP adapter
packages/atlas_mcp           MCP adapter (Planned)
packages/atlas_tui           Nocterm presentation package
apps/atlas_cli               atlas CLI, TUI, and other commands (planned `atlas server` subcommand)
apps/atlas_flutter           Flutter desktop and mobile application
```

The root Pub workspace owns the only `pubspec.lock`. Workspace members use `resolution: workspace` and must not add member lockfiles.

## Toolchain

The root `mise.toml` pins Flutter 3.47.0, which provides Dart 3.13.0.

```sh
mise install
mise run deps
```

Use `mise run deps-update` only when intentionally changing dependency constraints or the lockfile.

## Verification

```sh
mise run fmt          # format Dart sources
mise run fmt-check    # check formatting without rewriting
mise run analyze      # analyze the workspace
mise run test         # run available Dart and Flutter tests
mise run ci           # complete repository verification
```

Run the Flutter client with `mise run app-run --device macos`. Platform release builds use the matching `mise run app-build-*` task. Install a locally built macOS app into `/Applications` with `mise run app-install-macos` (override the destination with `APP_INSTALL_DIR`).

Build the single-file CLI binary with `mise run cli-build`. Dart 3.13's
`dart build cli` produces `build/bundle/bin/atlas`; packages with build hooks
(sqlite3) cannot use `dart compile exe`.

Install a locally built binary into `~/.local/bin` with `mise run cli-install`.
End users install a prebuilt release binary with
`curl -fsSL https://github.com/gvenusleo/atlas/releases/latest/download/install.sh | bash`
(macOS/Linux) or `irm .../latest/download/install.ps1 | iex` (Windows); the
scripts download the versioned artifact matching the platform and
architecture and honor `ATLAS_INSTALL_DIR`.

Releases are cut by pushing a `v*.*.*` tag:
`.github/workflows/release.yml` builds linux (amd64/arm64), macOS
(amd64/arm64), and Windows (amd64) binaries with `dart build cli` and uploads
them together with the install scripts to the GitHub release. Release notes
are generated automatically on the GitHub release page; the repository keeps
no separate changelog file.

## Package Rules

- Put domain concepts and runtime ports in `atlas_runtime`; keep provider, storage, tool, UI, and protocol implementations in their owning packages.
- Add public abstractions only when a real adapter or test requires them.
- Do not predeclare dependencies for planned code. Run `dart pub add` from the owning Dart package, or `flutter pub add` from `atlas_flutter`, when implementation code first needs a package.
- Use Dio for every HTTP request. Do not add `package:http` or a second HTTP client. Add a WebSocket dependency only with the first real `atlas_ws` implementation.
- Public Dart APIs require concise documentation comments.
- Runtime and protocol packages must not import Flutter.
- Presentation packages must not import provider, tool, or storage implementations.
- Application bootstrap code composes those adapters and injects the runtime.
  `atlas_cli` and `atlas_flutter` both call `atlas_composition`.
- `atlas_ws` owns WebSocket transport only and accepts an injected request handler.
- Generated serialization files stay beside their source and are committed only when the selected generator requires it.
- Add focused tests with behavior. Empty scaffold packages do not need placeholder tests.

## Documentation Rules

- Root README files contain product status and supported commands, not internal architecture.
- Architecture and dependency boundaries belong in `docs/architecture.md`.
- Mark unavailable behavior as `Planned`; remove stale examples when behavior is removed.
- Keep English and Chinese counterparts synchronized.

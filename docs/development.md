# Development

[中文](zh-CN/development.md)

## Workspace Layout

```text
packages/atlas_runtime       session and turn domain, timeline, ports, agent engine
packages/atlas_storage       Drift persistence and row mapping
packages/atlas_provider      model provider adapters
packages/atlas_config        JSON config loading and validation
packages/atlas_prompt        system prompt and skill catalog loading
packages/atlas_composition   shared runtime composition for CLI and Flutter
packages/atlas_tools         built-in tools
packages/atlas_ws            versioned WebSocket protocol and transport
packages/atlas_acp           ACP adapter
packages/atlas_mcp           MCP client tools (stdio and Streamable HTTP)
packages/atlas_tui           Nocterm presentation package
apps/atlas_cli               atlas CLI and TUI, with `atlas acp`, `atlas server`, and `atlas cache`
apps/atlas_flutter           Flutter desktop and mobile application
```

The root Pub workspace owns the only `pubspec.lock`; workspace members use `resolution: workspace` and must not add member lockfiles. Package responsibilities are listed in [Architecture](architecture.md).

## Toolchain

The root `mise.toml` pins Flutter 3.47.4, which provides Dart 3.13. Every package declares `sdk: ^3.13.0`, so the Dart 3.13 features the codebase uses (primary constructors, exhaustive switches over sealed types) need no experiment flag.

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

Run the Flutter client with `mise run app-run --device macos`. Platform release builds use the matching `mise run app-build-*` task, and `mise run app-install-macos` installs a locally built macOS app into `/Applications` (override with `APP_INSTALL_DIR`).

Build the single-file CLI with `mise run cli-build`. Dart 3.13's `dart build cli` writes `build/bundle/bin/atlas`; packages with build hooks (sqlite3) cannot use `dart compile exe`. `mise run cli-install` installs a locally built binary into `~/.local/bin`, while end users install a release binary with `install.sh` (macOS/Linux) or `install.ps1` (Windows); those scripts pick the artifact for the platform and architecture and honor `ATLAS_INSTALL_DIR`.

Pushing a `v*.*.*` tag releases a version: `.github/workflows/release.yml` builds linux (amd64/arm64), macOS (amd64/arm64), and Windows (amd64) binaries with `dart build cli` and uploads them with the install scripts. Release notes are generated on the GitHub release page; the repository keeps no changelog.

## CLI Contract

`bin/atlas.dart` forwards arguments and assigns the returned exit code. `--help`, `help <command>`, and `<command> --help` work without configuration, and argument errors are reported on stderr before any config load or storage open. Exit codes are 0 for success, 64 for invalid usage, 78 for configuration failures, and 70 for unexpected failures; `--verbose` adds a terse stack trace.

The default TUI needs terminal stdin/stdout, ANSI support, and an unset `NO_COLOR` (an empty value disables it). Unsupported terminals are rejected without escape sequences while non-interactive subcommands stay available, and TUI exit restores input modes and the cursor before releasing stdin. Commands close their resources and return naturally instead of calling `exit()`.

The CLI package declares the `atlas` executable, so `dart run atlas_cli:atlas --help` works from the workspace root. The version is generated from `apps/atlas_cli/pubspec.yaml` by `build_version`: after changing it, run `mise run cli-version` and commit `apps/atlas_cli/lib/src/version.dart`. `mise run cli-build` generates the file automatically, and release builds verify `--version` against the tag.

`mise run cli-integration-test` runs as part of `mise run ci`. It builds an isolated bundle under `.dart_tool/atlas_cli/` and checks real process output, exit codes, ACP EOF, and teardown; set `ATLAS_TEST_BINARY` to test an existing bundle instead. On macOS and Linux an FFI probe creates a real PTY and compares terminal state across exit, covering `/quit` with batched and fragmented input, signals, terminal restoration, and `NO_COLOR` (POSIX-only cases are skipped on Windows). Release jobs run the same suite on each built artifact.

### Reading `atlas cache`

`atlas cache --limit 200` samples the latest 200 turns and their persisted conversation responses, including history hidden by compaction, from a single database snapshot. Responses are grouped by provider/model and session; compaction summaries and attempts that produced no response record are excluded, so the report is not a billing ledger.

- **Token hit rate** is summed cache-read tokens over summed complete input tokens across measured requests; cache writes and output tokens do not count as hits.
- **Requests with hits** is the share of measured requests with any cache read, not the share with a fully cached prompt.
- **Cache-data coverage** is measured requests over recorded responses. Unknown, inconsistent, aborted, and zero-input records are disclosed separately and excluded from rates, and an unknown rate is `n/a` rather than a cache miss.

Provider adapters persist normalized input totals and cache-field availability next to the original counts, so old records stay readable without being reinterpreted from current configuration. Output is plain text, respects terminal width, and stays free of escape sequences when piped or when `NO_COLOR` is set.

## Package Rules

- Domain concepts and runtime ports belong in `atlas_runtime`; provider, storage, tool, UI, and protocol code belongs in its owning package.
- Add a public abstraction only when a real adapter or test needs it.
- Add dependencies with `dart pub add` in the package that owns the behavior, or `flutter pub add` in `atlas_flutter`; do not predeclare them at the workspace root.
- Use Dio for Atlas-owned HTTP requests. MCP integration uses `mcp_dart` and its `package:http` dependency for MCP transport and authentication; keep that exception inside the MCP adapter. Add a WebSocket dependency only when `atlas_ws` implements one.
- Public Dart APIs need concise documentation comments. Runtime and protocol packages must not import Flutter, and presentation packages must not import provider, tool, or storage implementations.
- Application bootstrap composes adapters and injects the runtime; both application roots use `atlas_composition`, and `atlas_ws` accepts an injected request handler.
- Generated serialization files stay beside their source and are committed only when the generator requires it. Add focused tests with behavior; empty scaffold packages need no placeholder tests.

Documentation rules live in the [documentation guide](README.md).

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
apps/atlas_cli               atlas CLI and TUI, with the `atlas acp`, `atlas server`, and `atlas cache` subcommands
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

## CLI Contract and Verification

`bin/atlas.dart` only forwards arguments and assigns the returned exit code.
`atlas --help`, `atlas help <command>`, and `<command> --help` work without
configuration. Argument errors are validated before loading configuration or
opening storage; both the error and usage go to stderr. Exit codes are 0 for
success, 64 for invalid usage, 78 for configuration failures, and 70 for
unexpected failures. `--verbose` includes a terse stack trace on stderr.

The default TUI requires terminal stdin/stdout, ANSI support, and an unset
`NO_COLOR` (even an empty value disables it). Unsupported terminals are rejected
without escape sequences; non-interactive subcommands remain available. TUI
exit restores input modes and the cursor before releasing stdin. Commands
close their resources and return naturally rather than calling `exit()`.

The CLI package declares the `atlas` executable; from the workspace root use
`dart run atlas_cli:atlas --help`. Its version is generated from
`apps/atlas_cli/pubspec.yaml` by `build_version`. Run `mise run cli-version`
after changing that version and commit `apps/atlas_cli/lib/src/version.dart`.
`mise run cli-build` generates it automatically; release builds verify that
`--version` matches the release tag before packaging.

`mise run cli-integration-test` is included in `mise run ci`. It builds an
isolated native bundle under `.dart_tool/atlas_cli/`, then checks real process
output, exit codes, ACP EOF, and teardown. Set `ATLAS_TEST_BINARY` to an absolute
executable path to test an existing bundle instead. On macOS/Linux, a Dart FFI
probe creates a real PTY and uses the system `stty` utility to compare terminal
state before and after exit. It tests `/quit`, signals, terminal restoration,
and `NO_COLOR` without Python. POSIX-only cases are skipped on Windows. Release
jobs run the same process suite against each platform's built artifact.

### Reading `atlas cache`

`atlas cache --limit 200` samples the latest 200 turns and all their persisted
conversation responses, including history hidden by context compaction. The
report is read from one database snapshot and groups responses by provider/model
and session. It does not include compaction-summary calls or attempts that never
produced a response record; it is not a complete billing ledger.

- **Token hit rate** is summed cache-read tokens divided by summed complete
  input tokens, over measured requests only. Cache writes and output tokens do
  not count as hits.
- **Requests with hits** is the share of measured requests with any cache read,
  not the share whose entire prompt was cached.
- **Cache-data coverage** is measured requests divided by recorded responses.
  Unknown, inconsistent, aborted, and zero-input records are excluded from rates
  and disclosed separately. An unknown rate is `n/a`, not a cache miss.
- Input breakdowns cover the same measured requests. When cache writes are not
  reported, the remaining input is **unclassified**, not assumed to be fresh.

Provider adapters persist normalized input totals and cache-field availability
alongside the original counts. Old records without this metadata remain
readable but are not reinterpreted using current configuration. Nonstandard
Chat Completions top-level cache buckets without standard `cached_tokens` have
unknown accounting and are excluded. New, standard usage gradually increases
coverage; no historical data is rewritten. Token hit rate is not cost savings,
and a low value alone does not identify the cause of a cache miss.

Output is plain text, respects terminal width, and remains escape-free when
piped or used with `NO_COLOR`. Session titles and provider names are sanitized
before rendering.

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

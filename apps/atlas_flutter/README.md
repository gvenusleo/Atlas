# Atlas Flutter

The desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable sidebars on wide windows, compact drawers on narrow windows, GitHub light and dark-dimmed palettes that follow the system theme, a local runtime bootstrap with sessions and agent turns, a file browser, an embedded terminal, and a remote connection screen that drives a computer-side `atlas server` over WebSocket (used on mobile, available on desktop through Settings). Remote sessions hide the local file browser and terminal: files and commands run on the computer. Every text field draws its caret through the shared `AnimatedCaret` wrapper, which animates the caret with spring-driven corner physics.

## Responsibility

- Composition root and ACP presentation client for desktop and mobile. The local app hosts an in-process ACP server; feature and presentation code renders UI only.
- Remote WebSocket mode: mobile boots into the remote connection screen and desktop can switch from Settings; connections and tokens live in the platform secure storage.

## Allowed dependencies

- Flutter SDK, `flutter_riverpod`, `go_router`, `window_manager`, `material_ui`, `lucide_icons_flutter`, `flutter_markdown_plus`, `file_picker`, `clipboard`, `pty2`, `terminal_view`, `flutter_secure_storage`, `shared_preferences`, `stream_channel`, and `web_socket_channel`.
- Tests may also import `atlas_ws` (dev dependency) to serve a real `atlas server` endpoint for remote connection integration tests.
- `atlas_composition` for process-level runtime construction.
- `atlas_config`, `atlas_prompt`, `atlas_provider`, and `atlas_storage` from application bootstrap only. Tests may also import `atlas_tools`.
- `atlas_runtime` public types for the injected runtime interface.

## Prohibited ownership

- No agent orchestration, provider logic, tool execution, or session persistence in feature or presentation code; only bootstrap composes adapters.
- No ACP protocol implementation; the app consumes `atlas_acp` as a client, including over WebSocket (the bridge lives in the app bootstrap). Local MCP tools are composed through `atlas_composition`; their protocol stays in `atlas_mcp`.
- No Nocterm rendering logic; the terminal TUI belongs to `atlas_tui`.

## Structure

```text
lib/main.dart                            bootstrap and ProviderScope
lib/app/bootstrap                        runtime bootstrap and process connectors
lib/app/routing, lib/app/platform        navigation and platform window
lib/features/<feature>/application       Riverpod controllers and immutable UI state
lib/features/<feature>/data              storage and platform services where needed
lib/features/<feature>/domain            shared models and ports where needed
lib/features/<feature>/presentation      pages, layouts, and widgets
lib/shared                               shared state, window UI, theme, and layout
```

Settings preferences use a feature repository backed by `shared_preferences`, seeded before the first frame. Connection controllers and saved-profile repositories live in `features/connections`; `app/bootstrap` injects the process and WebSocket connectors. File browsers use a Riverpod provider family keyed by session and directory; filesystem access lives in `features/files/data`. The workspace composes files and terminal hosts, while window controls live in `shared`. See [architecture](../../docs/architecture.md#flutter-client-state).

The client-local theme and language preferences live in `features/settings`: a repository handles storage, Riverpod controllers apply selections, and `atlas_app.dart` reads their state. The settings page (`/settings`) composes appearance and connection management; it shares window chrome with the workspace. Android and iOS register `atlas:///` for the workspace and `atlas:///settings` for settings. Settings retains the workspace beneath it when opened directly, so both the page back button and system back return to the workspace. HTTPS App Links / Universal Links remain Planned until a domain and Android release certificate fingerprints are supplied and the domain association files are hosted.

On macOS, startup imports exported variables from the user's interactive login shell once, so configuration values, local shell tools, and ACP subprocesses can find terminal-installed commands even when the app starts from Finder or Dock. Restart Atlas after editing shell configuration. Resolution failure falls back to the original environment; aliases and shell functions are not imported.

## Run and Verify

From the repository root:

```sh
mise run app-run --device macos
mise run ci
```

Platform release builds remain available through the `mise run app-build-*` tasks. Install a locally built macOS app with `mise run app-install-macos`.

With the app installed on an Android device or iOS simulator, check both a cold launch and delivery while the app is already open:

```sh
adb shell am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d 'atlas:///settings' xin.liuyu.atlas_app
xcrun simctl openurl booted 'atlas:///settings'
```

The three slashes keep `settings` in the URL path. Verify that settings opens and that its back button returns to the workspace; on Android also verify system back.

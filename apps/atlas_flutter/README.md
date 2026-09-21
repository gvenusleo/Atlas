# Atlas Flutter

The desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable
sidebars on wide windows, compact drawers on narrow windows, GitHub light and dark-dimmed
palettes that follow the system theme, a local runtime bootstrap with
sessions and agent turns, a file browser, an embedded terminal, and a remote
connection screen
that drives a computer-side `atlas server` over WebSocket (used on mobile,
available on desktop through Settings). Remote sessions hide the local file
browser and terminal: files and commands run on the computer. Every text
field draws its caret through the shared `AnimatedCaret` wrapper, which
animates the caret with spring-driven corner physics.

## Responsibility

- Composition root and ACP presentation client for desktop and mobile. The
  local app hosts an in-process ACP server; feature and presentation code
  renders UI only.
- Remote WebSocket mode: mobile boots into the remote connection screen and
  desktop can switch from Settings; connections and tokens live in the
  platform secure storage.

## Allowed dependencies

- Flutter SDK, `flutter_riverpod`, `go_router`, `window_manager`,
  `material_ui`, `lucide_icons_flutter`, `flutter_markdown_plus`,
  `file_picker`, `clipboard`, `pty2`, `terminal_view`,
  `flutter_secure_storage`, `stream_channel`, and `web_socket_channel`.
- Tests may also import `atlas_ws` (dev dependency) to serve a real
  `atlas server` endpoint for remote connection integration tests.
- `atlas_composition` for process-level runtime construction.
- `atlas_config`, `atlas_prompt`, and `atlas_storage` from application
  bootstrap only. Tests may also import `atlas_tools`.
- `atlas_runtime` public types for the injected runtime interface.

## Prohibited ownership

- No agent orchestration, provider logic, tool execution, or session
  persistence in feature or presentation code; only bootstrap composes
  adapters.
- No ACP protocol implementation; the app consumes `atlas_acp` as a client,
  including over WebSocket (the bridge lives in the app bootstrap). MCP
  adapters remain Planned.
- No Nocterm rendering logic; the terminal TUI belongs to `atlas_tui`.

## Structure

```text
lib/main.dart                            bootstrap and ProviderScope
lib/app                                  application root, routing, platform window, runtime bootstrap
lib/features/<feature>/application       feature controllers and state
lib/features/<feature>/data              local filesystem, terminal, and preference access
lib/features/<feature>/presentation      feature pages, layouts, and widgets
lib/shared                               shared application state, theme, and UI
```

Connection controllers and saved-profile repositories live in
`features/remote_connection`; `app` injects the process and WebSocket connectors.
The file browser delegates filesystem state and operations to its own Riverpod
controller. See [architecture](../../docs/architecture.md#flutter-client-state).

The client-local theme preference lives in `lib/app`: `theme_mode.dart` reads and
persists the theme mode with `shared_preferences`, and `atlas_app.dart`
applies it. The settings page (`/settings`) owns appearance preferences and
ACP connection management, and reuses the workspace window chrome.
Android and iOS register `atlas:///` for the workspace and `atlas:///settings`
for settings. Settings retains the workspace beneath it when opened directly,
so both the page back button and system back return to the workspace.
HTTPS App Links / Universal Links remain Planned until a domain and Android
release certificate fingerprints are supplied and the domain association files
are hosted.

## Run and Verify

From the repository root:

```sh
mise run app-run --device macos
mise run ci
```

Platform release builds remain available through the `mise run app-build-*` tasks. Install a locally built macOS app with `mise run app-install-macos`.

With the app installed on an Android device or iOS simulator, check both a cold
launch and delivery while the app is already open:

```sh
adb shell am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d 'atlas:///settings' xin.liuyu.atlas_app
xcrun simctl openurl booted 'atlas:///settings'
```

The three slashes keep `settings` in the URL path. Verify that settings opens and
that its back button returns to the workspace; on Android also verify system back.

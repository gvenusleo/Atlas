# Atlas Flutter

The desktop and mobile client for Atlas.

## Responsibility

- Composition root and ACP presentation client for desktop and mobile: the local app hosts an in-process ACP server, and feature and presentation code renders UI only.
- Remote WebSocket mode: mobile boots into the remote connection screen while desktop switches from Settings, with connections and tokens in platform secure storage. Remote sessions hide the local file browser and terminal, because files and commands run on the computer.
- Provides the responsive workspace shell: resizable sidebars on wide windows, drawers on narrow ones, and themes that follow the system appearance.

## Allowed dependencies

- Flutter SDK, `flutter_riverpod`, `go_router`, `window_manager`, `material_ui`, `lucide_icons_flutter`, `flutter_markdown_plus`, `file_picker`, `clipboard`, `pty2`, `terminal_view`, `flutter_secure_storage`, `shared_preferences`, `stream_channel`, and `web_socket_channel`.
- `atlas_composition` for process-level runtime construction, and `atlas_config`, `atlas_prompt`, `atlas_provider`, and `atlas_storage` from application bootstrap only.
- `atlas_runtime` public types for the injected runtime interface. Tests may also import `atlas_ws` (dev dependency) to serve a real `atlas server` endpoint, and `atlas_tools`.

## Prohibited ownership

- No agent orchestration, provider logic, tool execution, or session persistence in feature or presentation code; only bootstrap composes adapters.
- No ACP protocol implementation; the app consumes `atlas_acp` as a client, and local MCP tools are composed through `atlas_composition`.
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

Each feature owns its controllers, and `app/bootstrap` injects the process and WebSocket connectors. Settings preferences are seeded before the first frame, file browsers are keyed by session and directory, and the workspace composes the files and terminal hosts. See [Flutter client state](../../docs/architecture.md#flutter-client-state).

The `go_router` tree nests `/settings` under `/`, so opening settings directly keeps a workspace to return to, and Android and iOS register `atlas:///` and `atlas:///settings`. HTTPS App Links and Universal Links remain Planned until a domain, release certificate fingerprints, and hosted association files exist. On macOS, startup imports the user's exported login-shell variables once so configuration values, shell tools, and ACP subprocesses can find terminal-installed commands when the app starts from Finder or Dock; restart after editing shell configuration, and note that the snapshot contains no aliases or functions.

## Run and Verify

From the repository root:

```sh
mise run app-run --device macos
mise run ci
```

Platform release builds use the `mise run app-build-*` tasks, and `mise run app-install-macos` installs a locally built macOS app. With the app installed on an Android device or iOS simulator, check deep links on a cold launch and while the app is already open:

```sh
adb shell am start -a android.intent.action.VIEW -c android.intent.category.BROWSABLE -d 'atlas:///settings' xin.liuyu.atlas_app
xcrun simctl openurl booted 'atlas:///settings'
```

Settings must open with a working back button, and on Android system back must also return to the workspace.

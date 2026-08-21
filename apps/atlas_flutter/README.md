# Atlas Flutter

The desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable
desktop sidebars, compact mobile drawers, Ayu light and dark palettes that
follow the system theme, a local runtime bootstrap with sessions and agent
turns, a file browser, and an embedded terminal. Remote WebSocket mode is
not implemented yet.

## Responsibility

- Composition root and presentation client for desktop and mobile. The local
  app receives the shared runtime directly: application bootstrap may compose
  runtime adapters and inject the runtime; feature and presentation code
  renders UI only.
- Remote WebSocket mode is planned separately through `atlas_ws`.

## Allowed dependencies

- Flutter SDK, `flutter_riverpod`, `go_router`, `window_manager`,
  `material_ui`, `lucide_icons_flutter`, `flutter_markdown_plus`,
  `file_selector`, `super_clipboard`, `pty2`, and `terminal_view`.
- `atlas_composition` for process-level runtime construction.
- `atlas_config`, `atlas_prompt`, and `atlas_storage` from application
  bootstrap only. Tests may also import `atlas_tools`.
- `atlas_runtime` public types for the injected runtime interface.

## Prohibited ownership

- No agent orchestration, provider logic, tool execution, or session
  persistence in feature or presentation code; only bootstrap composes
  adapters.
- No remote client protocol logic; `atlas_ws`, `atlas_acp`, and `atlas_mcp`
  are not owned here.
- No Nocterm rendering logic; the terminal TUI belongs to `atlas_tui`.

## Structure

```text
lib/main.dart                            bootstrap and ProviderScope
lib/app                                  application root, routing, platform window, runtime bootstrap
lib/features/<feature>/application       feature controllers and state
lib/features/<feature>/data              local filesystem and terminal access
lib/features/<feature>/presentation      feature pages, layouts, and widgets
lib/shared                               application-wide theme and shared UI
```

## Run and Verify

From the repository root:

```sh
mise run app-run --device macos
mise run ci
```

Platform debug builds remain available through the `mise run app-build-*` tasks.

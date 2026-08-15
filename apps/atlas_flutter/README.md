# Atlas Flutter

The desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable
desktop sidebars, compact mobile drawers, and system light and dark themes.
Runtime connectivity, sessions, and agent turns are not implemented yet.

## Responsibility

- Composition root and presentation client for desktop and mobile. The local
  app receives the shared runtime directly: application bootstrap may compose
  runtime adapters and inject the runtime; feature and presentation code
  renders UI only.
- Remote WebSocket mode is planned separately through `atlas_ws`.

## Allowed dependencies

- Flutter SDK, `flutter_riverpod`, `go_router`, `window_manager`, and the
  `material_ui` / `cupertino_ui` design libraries.
- `atlas_runtime` public types for the injected runtime interface (added when
  connectivity lands).

## Prohibited ownership

- No agent orchestration, provider logic, tool execution, or session
  persistence in feature or presentation code; only bootstrap composes
  adapters.
- No remote client protocol logic; `atlas_ws`, `atlas_acp`, and `atlas_mcp`
  are not owned here.
- No terminal rendering logic; the Nocterm UI belongs to `atlas_tui`.

## Structure

```text
lib/main.dart                         bootstrap and ProviderScope
lib/app                               application root, routing, platform window
lib/features/<feature>/presentation  feature pages, layouts, and widgets
lib/shared                            application-wide theme and shared UI
```

## Run and Verify

From the repository root:

```sh
mise run app-run --device macos
mise run ci
```

Platform debug builds remain available through the `mise run app-build-*` tasks.

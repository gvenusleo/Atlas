# Atlas Flutter

The desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable desktop sidebars, compact mobile drawers, and system light and dark themes. Runtime connectivity, sessions, and agent turns are not implemented yet.

The local app will receive the shared runtime directly. Application bootstrap
may compose runtime adapters, while feature and presentation code must not own
agent orchestration, providers, tools, or persistence. Remote WebSocket mode is
planned separately through `atlas_ws`.

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
just app-run macos
just ci
```

Platform debug builds remain available through the `just app-build-*` recipes.

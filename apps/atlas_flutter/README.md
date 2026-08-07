# Atlas Flutter

The desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable desktop sidebars, compact mobile drawers, and system light and dark themes. Runtime connectivity, sessions, and agent turns are not implemented yet.

The app will consume the versioned `atlas_protocol`; it must not implement agent orchestration, providers, tools, or persistence.

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

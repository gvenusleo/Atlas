# Atlas App

Flutter desktop and mobile client for Atlas.

## Status

The current implementation provides a responsive workspace shell, resizable
desktop sidebars, compact mobile drawers, and system light and dark themes.
Runtime features are not connected yet, so the app does not currently create
sessions or run agent turns.

## Structure

```text
lib/main.dart                         bootstrap and ProviderScope
lib/app                               application root, routing, platform window
lib/features/<feature>/presentation  feature pages, layouts, and widgets
lib/shared                            application-wide theme and shared UI
```

Page-level navigation uses go_router. Riverpod owns application-wide,
asynchronous, and cross-page state; transient visual state stays with the
widget that renders it. Future runtime integration will use Atlas's WebSocket
channel instead of duplicating agent behavior in Dart.

## Run

```sh
fvm flutter run -d macos
```

## Verify

```sh
fvm flutter analyze
fvm flutter test
fvm flutter build macos --debug
```

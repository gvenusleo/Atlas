# Contributing to Atlas

Atlas is currently a Dart and Flutter workspace scaffold. Check the current implementation before documenting or depending on a planned capability.

## Development Setup

Prerequisites: Git, [mise](https://mise.jdx.dev/), and [just](https://github.com/casey/just).

```sh
git clone https://github.com/gvenusleo/atlas.git
cd atlas
mise install
just deps
just ci
```

Use `just deps-update` only when intentionally changing dependencies. The workspace has one root `pubspec.lock`.

## Change Boundaries

- Keep the agent loop in `atlas_runtime`; adapters and clients must not duplicate it.
- Keep Flutter and Nocterm independent of providers, tools, and persistence.
- Add dependencies in the package that owns the behavior, not at the workspace root.
- Do not create placeholder abstractions for planned features.
- Mark unimplemented behavior as `Planned` in documentation.
- Keep English and Chinese documents synchronized when a translated counterpart exists.

Package responsibilities and dependency direction are defined in [Architecture](docs/architecture.md). Commands and verification are documented in [Development](docs/development.md).

## Pull Requests

Open an issue before large architectural changes, public protocol changes, persistent schema changes, or new provider adapters. Reproducible bug fixes and focused documentation corrections can be submitted directly.

Before submitting, run:

```sh
just ci
```

Use Conventional Commits, for example:

```text
feat(runtime): add run cancellation
fix(protocol): preserve event ordering
docs: clarify workspace boundaries
```

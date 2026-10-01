# Contributing to Atlas

Check the current implementation before documenting or depending on a planned capability.

## Setup

Prerequisites are Git and [mise](https://mise.jdx.dev/).

```sh
git clone https://github.com/gvenusleo/atlas.git
cd atlas
mise install
mise run deps
mise run ci
```

The workspace has one root `pubspec.lock`; use `mise run deps-update` only when intentionally changing dependencies. Commands and verification are documented in [Development](docs/development.md), and package responsibilities in [Architecture](docs/architecture.md).

## Pull Requests

Open an issue before large architectural changes, public protocol changes, persistent schema changes, or new provider adapters. Reproducible bug fixes and focused documentation corrections can be submitted directly. Run `mise run ci` before submitting, and keep English and Chinese documents synchronized when a translated counterpart exists.

Use Conventional Commits:

```text
feat(runtime): add run cancellation
fix(protocol): preserve event ordering
docs: clarify workspace boundaries
```

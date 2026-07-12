# Contributing to Atlas

Thanks for your interest in contributing to Atlas.

## Issues vs PRs

If the problem is easy to reproduce, prefer opening a GitHub issue. A clear issue should include: reproduction steps, expected behavior, actual behavior, logs or screenshots.

PRs are more useful when the problem depends on a specific environment (e.g. macOS/Windows-specific behavior, particular shells or filesystems), because they capture the behavior in the environment where the problem actually occurs.

## When you can open a PR directly

- Reproducible bug fixes with a focused diff
- Documentation fixes, typos
- Small changes that clearly match an existing issue

## When to discuss first

Open an issue first to align before investing time in a PR:

- New features or user-visible behavior changes
- Architectural changes or refactors larger than ~100 lines
- Public API or configuration format changes
- New provider adapters

## Development Setup

Prerequisites: Go 1.26+, Git, [just](https://github.com/casey/just).

```sh
git clone https://github.com/gvenusleo/atlas.git
cd atlas
go build ./...                # build
go test ./...                 # run all tests
go test ./internal/agent/...  # run a single package's tests
```

Before submitting a change, run `just ci` to check formatting, module files, builds, vet, race tests, and release-target cross-builds.

## Commit Convention

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add MCP support
fix(acp): fix cwd lost on session restore
docs: update README configuration guide
chore(release): bump version to 0.8.0
```

## Code Principles

Atlas aims to be small, clear, and verifiable. Before submitting, confirm:

- **Minimal changes**: only touch code directly related to the task. No drive-by refactors.
- **Testable**: cover key behavior with fake providers and temp directories.
- **No premature abstraction**: don't abstract before two real call sites exist. Don't keep interfaces for "maybe later."
- **Single core**: CLI, ACP, and WebSocket share the same `runtime.Runtime`. Channel layers only do protocol adaptation.

See [AGENTS.md](AGENTS.md) for full design principles.

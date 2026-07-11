# Atlas Development Guide

## Product Boundaries

- Atlas is a local general-purpose agent. Its tools have the same filesystem and shell permissions as the Atlas process.
- Atlas does not provide a sandbox, permission prompts, or an approval gate. Do not introduce permission abstractions unless the product direction changes.
- All channels use the shared runtime and agent loop. Channel packages only adapt protocols and manage channel-specific state; they must not duplicate the agent loop.
- `model.Provider` is the only interface to model backends. Provider adapters own connection settings, authentication, provider-specific request formats, and response conversion. Provider connection fields must not enter `model.ChatRequest`.
- ACP `run_shell` may execute through a client terminal, while `apply_patch` always modifies the filesystem visible to the Atlas process. Remote ACP workspaces where those filesystems differ are not supported.

## Change Constraints

- State assumptions when requirements are ambiguous. Ask before proceeding when different interpretations would materially change the result.
- Make the smallest change that fully solves the request. Do not perform unrelated refactors, formatting, cleanup, or speculative improvements.
- Preserve existing style and package boundaries. Do not add abstractions, compatibility layers, fallbacks, or config switches before there are real call sites or provider differences that require them.
- Prefer the Go standard library. Remove only code made unused by the current change.
- Keep the agent loop predictable: every tool call has a paired result, tool results preserve model order, tool errors are written back to the transcript, and observer events preserve occurrence order.
- Public interfaces accept `context.Context`. Exported packages, types, and functions require concise Go doc comments; other comments should explain only non-obvious behavior.
- Keep provider-specific behavior in `internal/provider/*`, orchestration and persistence in `internal/runtime`, protocol adaptation in channel packages, and tool execution behind `tool.Tool`.
- Read relevant files before editing. Treat current code, tests, and command output as the source of truth rather than relying on documentation or assumptions.

## Verification

- Run focused tests for the changed behavior first.
- Before delivery, run:

```sh
just ci
```

- Report the verification commands that passed and any remaining risk. Do not treat command completion alone as proof; verify the observable result of file or behavior changes.

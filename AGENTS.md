# Atlas Development Guide

## Product Boundaries

- Atlas is a local general-purpose agent. Its tools have the same filesystem and shell permissions as the Atlas process.
- Atlas does not provide a sandbox, permission prompts, or an approval gate. Do not introduce permission abstractions unless the product direction changes.
- All channels use the shared runtime and agent loop. Channel packages only adapt protocols and manage channel-specific state; they must not duplicate the agent loop.
- `app/` is the Flutter desktop and mobile client. It currently provides the application shell only and is not connected to the Atlas runtime.
- When runtime integration is added, the Flutter client must use the existing WebSocket channel rather than reimplementing the agent loop, tools, provider calls, or session persistence in Dart.
- `model.Provider` is the only interface to model backends. Provider adapters own connection settings, authentication, provider-specific request formats, and response conversion. Provider connection fields must not enter `model.ChatRequest`.
- ACP `run_shell` may execute through a client terminal, but calls with non-empty `stdin` execute through the Atlas process because ACP terminals do not accept standard input. Remote ACP workspaces where those filesystems differ are not supported.

## Change Constraints

- State assumptions when requirements are ambiguous. Ask before proceeding when different interpretations would materially change the result.
- Make the smallest change that fully solves the request. Do not perform unrelated refactors, formatting, cleanup, or speculative improvements.
- Preserve existing style and package boundaries. Do not add abstractions, compatibility layers, fallbacks, or config switches before there are real call sites or provider differences that require them.
- Prefer the Go standard library. Remove only code made unused by the current change.
- Keep the agent loop predictable: every tool call has a paired result, tool results preserve model order, tool errors are written back to the transcript, and observer events preserve occurrence order.
- Public interfaces accept `context.Context`. Exported packages, types, and functions require concise Go doc comments; other comments should explain only non-obvious behavior.
- Keep provider-specific behavior in `internal/provider/*`, orchestration and persistence in `internal/runtime`, protocol adaptation in channel packages, and tool execution behind `tool.Tool`.
- Read relevant files before editing. Treat current code, tests, and command output as the source of truth rather than relying on documentation or assumptions.

## Flutter App

- Keep bootstrap, routing, and platform-window integration in `app/lib/app`; reusable application-wide UI and theme code belongs in `app/lib/shared`.
- Organize product code by feature under `app/lib/features/<feature>`. Add `domain` or `data` layers only when a feature has real business logic or external data access that requires them.
- Use Riverpod for application-wide, asynchronous, or cross-page state. Keep transient presentation state such as hover, animation, drawer visibility, and panel width inside the owning widget.
- Use go_router for page-level navigation. Direct `Navigator` calls are acceptable for local UI surfaces such as drawers, dialogs, and sheets.
- Preserve dependency direction: `app` may depend on `features` and `shared`; features may depend on `shared`; `shared` must not import a feature.
- Do not place agent orchestration, model-provider logic, tool execution, or session persistence in Flutter. Those capabilities remain owned by the Go runtime and are exposed through channels.
- Public Dart types and functions require concise documentation comments. Follow the existing responsive shell and adaptive theme conventions unless the requested change explicitly alters them.

## Verification

- Run focused tests for the changed behavior first.
- For changes under `app/`, run `fvm flutter analyze` and `fvm flutter test` from `app/`. Also run `fvm flutter build macos --debug` when desktop window integration or macOS-specific behavior changes.
- Before delivery, run:

```sh
just ci
```

- Report the verification commands that passed and any remaining risk. Do not treat command completion alone as proof; verify the observable result of file or behavior changes.

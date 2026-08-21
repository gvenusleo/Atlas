# Atlas Development Guide

## Product Boundaries

- Atlas is a local general-purpose agent. Its tools have the same filesystem and shell permissions as the Atlas process.
- Atlas does not provide a sandbox, permission prompts, or an approval gate. Do not introduce permission abstractions unless the product direction changes.
- The repository is being rebuilt as a Dart and Flutter workspace. The agent runtime, CLI, ACP adapter, Nocterm TUI, and Flutter local client are implemented; the WebSocket transport and MCP remain unimplemented.
- All clients and protocol adapters must use the single runtime in `packages/atlas_runtime`. They must not duplicate the agent loop.
- Local Flutter and Nocterm entry points receive the runtime directly. Remote clients use the versioned WebSocket contract in `atlas_ws`.
- Presentation code must not call model providers, tools, or storage directly. Application bootstrap code may construct those adapters and inject the shared runtime.
- Provider-specific authentication, endpoints, request fields, and response conversion belong in `packages/atlas_provider` and must not enter `atlas_runtime` domain requests.

## Change Constraints

- State assumptions when requirements are ambiguous. Ask before proceeding when different interpretations would materially change the result.
- Make the smallest change that fully solves the request. Do not perform unrelated refactors, formatting, cleanup, or speculative improvements.
- Preserve package boundaries. Add an abstraction only when real call sites require it.
- Do not predeclare package dependencies. Add a dependency from the owning package with `dart pub add` only when implementation code first imports it; use `flutter pub add` for the Flutter application.
- Use Dio for all HTTP requests. Do not add `package:http` or another HTTP client. Add a dedicated WebSocket dependency only when `atlas_ws` contains an implementation that requires it.
- Keep the agent loop predictable: every tool call has a paired result, tool results preserve model order, errors are model-visible, and emitted events preserve occurrence order.
- Persisted timeline items must belong to the same session and turn; storage writes that update multiple records must be atomic.
- The runtime serializes active turns per session, and persisted provider/tool failures use safe summaries rather than raw exception text.
- Compaction checkpoints end at the final timeline item of a terminal turn; they must never split an assistant/tool/result group.
- Public Dart types and functions require concise documentation comments. Other comments should explain only non-obvious behavior.
- Read relevant files before editing. Current code and command output are the source of truth.

## Workspace Boundaries

- `atlas_runtime` owns domain models, events, ports, orchestration, cancellation, compaction, skills, and the model/tool loop. It must not depend on persistence, provider, tool, UI, or protocol implementations.
- `atlas_storage`, `atlas_provider`, and `atlas_tools` depend on and implement runtime ports without owning orchestration.
- `atlas_ws` owns versioned WebSocket wire DTOs, codecs, client and server transport behavior, and explicit conversion to runtime types. It may depend on `atlas_runtime` but must not compose runtime services.
- `atlas_acp` and `atlas_mcp` use `json_rpc_2` directly and own their different lifecycle and transport rules. Extract shared RPC code only after stable duplication exists.
- `atlas_acp` and `atlas_mcp` adapt protocols to the shared runtime.
- `atlas_tui` renders and interacts with an injected runtime interface; it does not depend on remote client protocols.
- `atlas_cli` and `atlas_flutter` are application composition roots. Running `atlas` enters the TUI by default; `atlas acp` serves the composed runtime to ACP clients; a planned `atlas server` will expose the composed runtime through `atlas_ws`.

## Flutter App

- Keep bootstrap, routing, and platform-window integration in `apps/atlas_flutter/lib/app`; reusable application-wide UI and theme code belongs in `lib/shared`.
- Organize product code by feature under `lib/features/<feature>`. Add `domain` or `data` layers only when a feature has real business logic or external data access.
- Use Riverpod for application-wide, asynchronous, or cross-page state. Keep transient presentation state in the owning widget.
- Use go_router for page-level navigation. Direct `Navigator` calls are acceptable for dialogs, sheets, drawers, and other local UI surfaces.
- Preserve dependency direction: `app` may depend on features and shared code; features may depend on shared code; shared code must not import a feature.
- Flutter bootstrap may compose runtime adapters, but Flutter feature and presentation code must not own agent orchestration, provider logic, tool execution, or session persistence.

## Documentation

- Root README files describe the product, current status, and supported entry points; architecture details belong in `docs/architecture.md`.
- English documents define structure and terminology. Keep the corresponding `docs/zh-CN` translation synchronized in the same change.
- Mark unimplemented behavior as `Planned`. Do not document planned commands or configuration as currently available.
- Every workspace package README states its responsibility, allowed dependencies, and prohibited ownership.

## Verification

- Run focused tests for changed behavior first.
- Run `mise run ci` before delivery. It resolves the locked workspace, checks formatting, analyzes Dart and Flutter code, and runs available tests.
- For Flutter platform integration changes, also run the matching `mise run app-build-*` task.
- Build the CLI with `mise run cli-build` (`dart build cli`, required for packages with build hooks like sqlite3; `dart compile exe` cannot link them). The binary lands at `build/bundle/bin/atlas`; do not recreate a `build/atlas` symlink.
- Report commands that passed and remaining risk. Command completion alone is not proof; verify the observable file or behavior change.

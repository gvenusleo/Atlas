# Atlas Development Guide

## Product Boundaries

- Atlas is a local general-purpose agent. Its tools have the same filesystem and shell permissions as the Atlas process.
- Atlas does not provide a sandbox, permission prompts, or an approval gate. Do not introduce permission abstractions unless the product direction changes.
- The repository is being rebuilt as a Dart and Flutter workspace. The Flutter application shell exists; the agent runtime, daemon, CLI, ACP, MCP, and Nocterm clients are not implemented yet.
- All clients and protocol adapters must use the single runtime in `packages/atlas_runtime`. They must not duplicate the agent loop.
- Flutter and Nocterm are clients of the versioned `atlas_protocol`. They must not call model providers, tools, or storage directly.
- Provider-specific authentication, endpoints, request fields, and response conversion belong in `packages/atlas_provider` and must not enter `atlas_core` domain requests.

## Change Constraints

- State assumptions when requirements are ambiguous. Ask before proceeding when different interpretations would materially change the result.
- Make the smallest change that fully solves the request. Do not perform unrelated refactors, formatting, cleanup, or speculative improvements.
- Preserve package boundaries. Add an abstraction only when real call sites require it.
- Keep the future agent loop predictable: every tool call has a paired result, tool results preserve model order, errors are model-visible, and emitted events preserve occurrence order.
- Public Dart types and functions require concise documentation comments. Other comments should explain only non-obvious behavior.
- Read relevant files before editing. Current code and command output are the source of truth.

## Workspace Boundaries

- `atlas_core` contains stable domain models, events, and ports. It must not depend on Flutter, persistence, providers, tools, or protocol transports.
- `atlas_runtime` owns orchestration, cancellation, compaction, skills, and the model/tool loop.
- `atlas_storage`, `atlas_provider`, and `atlas_tools` implement ports without owning orchestration.
- `atlas_rpc` contains generic JSON-RPC behavior; ACP- and MCP-specific lifecycle rules remain in their protocol packages.
- `atlas_protocol` contains client wire DTOs and must not expose persistence or provider-specific models.
- `atlas_acp` and `atlas_mcp` adapt protocols to the shared runtime.
- `atlas_tui` and `atlas_flutter` are presentation clients of `atlas_protocol`.
- `atlasd` is the composition root for runtime services. `atlas_cli` is the command-line and terminal entry point.

## Flutter App

- Keep bootstrap, routing, and platform-window integration in `apps/atlas_flutter/lib/app`; reusable application-wide UI and theme code belongs in `lib/shared`.
- Organize product code by feature under `lib/features/<feature>`. Add `domain` or `data` layers only when a feature has real business logic or external data access.
- Use Riverpod for application-wide, asynchronous, or cross-page state. Keep transient presentation state in the owning widget.
- Use go_router for page-level navigation. Direct `Navigator` calls are acceptable for dialogs, sheets, drawers, and other local UI surfaces.
- Preserve dependency direction: `app` may depend on features and shared code; features may depend on shared code; shared code must not import a feature.
- Do not place agent orchestration, provider logic, tool execution, or session persistence in Flutter.

## Documentation

- Root README files describe the product, current status, and supported entry points; architecture details belong in `docs/architecture.md`.
- English documents define structure and terminology. Keep the corresponding `docs/zh-CN` translation synchronized in the same change.
- Mark unimplemented behavior as `Planned`. Do not document planned commands or configuration as currently available.
- Record high-impact and difficult-to-reverse decisions under `docs/decisions`.
- Every workspace package README states its responsibility, allowed dependencies, and prohibited ownership.

## Verification

- Run focused tests for changed behavior first.
- Run `just ci` before delivery. It resolves the locked workspace, checks formatting, analyzes Dart and Flutter code, and runs available tests.
- For Flutter platform integration changes, also run the matching `just app-build-*` recipe.
- Report commands that passed and remaining risk. Command completion alone is not proof; verify the observable file or behavior change.
